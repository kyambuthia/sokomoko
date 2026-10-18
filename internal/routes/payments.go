package routes

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/payments"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
)

type PaymentPageData struct {
	Title     string
	CSRFToken string
	Viewer    Viewer
	Flash     Flash
	Payment   paymentsvc.StatusView
	Phone     string
}

func orderIDFromPath(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	return id, err == nil && id > 0
}

// PaymentPage shows an order's online payment and lets the customer start or
// retry it. GET renders the state; POST starts a new attempt.
func PaymentPage(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := requestUserID(r)
		orderID, ok := orderIDFromPath(r)
		if !ok {
			NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		case http.MethodPost:
			outcome, err := a.Payment.Start(r.Context(), paymentsvc.StartRequest{
				UserID: userID, OrderID: orderID, Phone: r.FormValue("phone"), BaseURL: a.PublicBase(r),
			})
			switch {
			case err == nil && outcome.RedirectURL != "":
				http.Redirect(w, r, outcome.RedirectURL, http.StatusSeeOther)
				return
			case errors.Is(err, db.ErrPaymentNotFound):
				NotFound(w, r)
				return
			case err != nil:
				log.Printf("retry payment for order %d: %v", orderID, err)
				if errors.Is(err, paymentsvc.ErrNoPublicURL) {
					redirectWithFlash(w, r, r.URL.Path, "danger", "Online payments are not available right now.")
					return
				}
				if errors.Is(err, paymentsvc.ErrNotPayable) {
					redirectWithFlash(w, r, r.URL.Path, "danger", "This order cannot be paid online.")
					return
				}
			}
			http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
			return
		default:
			methodNotAllowed(w, http.MethodGet, http.MethodHead, http.MethodPost)
			return
		}

		view, err := a.Payment.Status(r.Context(), userID, orderID)
		if errors.Is(err, db.ErrPaymentNotFound) {
			NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("payment status for order %d: %v", orderID, err)
			serverError(w, r)
			return
		}
		switch view.Status {
		case "paid":
			redirectWithFlash(w, r, fmt.Sprintf("/account#order-%d", orderID), "success", fmt.Sprintf("Payment received for order #%d", orderID))
			return
		case "offline":
			http.Redirect(w, r, fmt.Sprintf("/account#order-%d", orderID), http.StatusSeeOther)
			return
		}
		a.Render(w, a.Templates.Payment, PaymentPageData{
			Title:     "Pay",
			CSRFToken: a.Auth.CSRFToken(r),
			Viewer:    viewerFromRequest(r),
			Flash:     popFlash(w, r),
			Payment:   view,
			Phone:     displayPhone(view.Phone),
		})
	}
}

// displayPhone turns 254712345678 back into 0712 345 678 for the form.
func displayPhone(normalized string) string {
	if len(normalized) == 12 && strings.HasPrefix(normalized, "254") {
		local := "0" + normalized[3:]
		return local[:4] + " " + local[4:7] + " " + local[7:]
	}
	return normalized
}

// PaymentReturn is where hosted payment pages (Paystack) send the customer
// back. The payment is confirmed with the provider before it counts.
func PaymentReturn(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		userID, _ := requestUserID(r)
		reference := strings.TrimSpace(r.URL.Query().Get("reference"))
		if reference == "" {
			reference = strings.TrimSpace(r.URL.Query().Get("trxref"))
		}
		orderID, err := a.Payment.HandleReturn(r.Context(), userID, reference)
		if errors.Is(err, db.ErrPaymentNotFound) {
			NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("payment return %q: %v", reference, err)
		}
		http.Redirect(w, r, fmt.Sprintf("/payments/order/%d", orderID), http.StatusSeeOther)
	}
}

func readWebhookBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

// PaymentWebhook receives signed provider events (Paystack). It answers 200
// once an event is processed or deliberately ignored, so the provider stops
// retrying, and 5xx on transient failures so it retries later.
func PaymentWebhook(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := readWebhookBody(w, r)
		if !ok {
			return
		}
		provider := r.PathValue("provider")
		err := a.Payment.HandleWebhook(r.Context(), provider, r, body)
		switch {
		case err == nil, errors.Is(err, payments.ErrUnsupported), errors.Is(err, db.ErrPaymentNotFound):
			w.WriteHeader(http.StatusOK)
		case errors.Is(err, payments.ErrBadSignature):
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
		case errors.Is(err, paymentsvc.ErrUnknownProvider), errors.Is(err, payments.ErrNotConfigured):
			NotFound(w, r)
		default:
			log.Printf("payment webhook %s: %v", provider, err)
			http.Error(w, "Temporarily unavailable", http.StatusServiceUnavailable)
		}
	}
}

// PaymentCallback receives unsigned provider callbacks (M-Pesa Daraja) at a
// per-payment secret URL. Daraja expects a JSON acknowledgement.
func PaymentCallback(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, ok := readWebhookBody(w, r)
		if !ok {
			return
		}
		provider, token := r.PathValue("provider"), r.PathValue("token")
		err := a.Payment.HandleCallback(r.Context(), provider, token, r, body)
		switch {
		case err == nil:
		case errors.Is(err, db.ErrPaymentNotFound), errors.Is(err, paymentsvc.ErrUnknownProvider), errors.Is(err, payments.ErrNotConfigured):
			NotFound(w, r)
			return
		default:
			log.Printf("payment callback %s: %v", provider, err)
			http.Error(w, "Temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		app.JSON(w, http.StatusOK, map[string]any{"ResultCode": 0, "ResultDesc": "Accepted"})
	}
}
