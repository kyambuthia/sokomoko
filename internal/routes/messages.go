package routes

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/domain"
	messagingSvc "github.com/kyambuthia/sokomoko/internal/service/messaging"
)

type MessagesPageData struct {
	Title, Role, Username, Workspace, Mode, CSRFToken, Error, Body, Nonce string
	Viewer                                                                Viewer
	Flash                                                                 Flash
	Inbox                                                                 messagingSvc.ChatInbox
	Contacts                                                              []messagingSvc.ChatContact
	Target                                                                messagingSvc.ChatTarget
	Thread                                                                messagingSvc.ChatThread
	LastMessageID                                                         int
	OlderPage                                                             bool
	Encrypted                                                             bool
	UserID                                                                int
}

func registerMessaging(a *app.App, mux *http.ServeMux, workspace string, roles []string) {
	wrap := func(h http.HandlerFunc) http.Handler { return withAnyRole(a, roles, h) }
	mux.Handle("/messages", wrap(messageInbox(a, workspace)))
	mux.Handle("/messages/new", wrap(messageCompose(a, workspace)))
	mux.Handle("/messages/start", wrap(messageStart(a, workspace)))
	mux.Handle("/messages/{id}", wrap(messageThread(a, workspace)))
	mux.Handle("/messages/{id}/send", wrap(messageSend(a, workspace)))
	mux.Handle("/messages/{id}/action", wrap(messageAction(a)))
}

func messagingActor(r *http.Request) *messagingSvc.Actor {
	u := auth.GetUserFromContext(r.Context())
	if u == nil {
		return nil
	}
	return &messagingSvc.Actor{ID: u.ID, Role: u.Role}
}

func messagesPage(a *app.App, r *http.Request, workspace, mode string) MessagesPageData {
	p := MessagesPageData{Title: "Messages", Workspace: workspace, Mode: mode, CSRFToken: a.Auth.CSRFToken(r), Viewer: viewerFromRequest(r)}
	if u := auth.GetUserFromContext(r.Context()); u != nil {
		p.Username, p.Role, p.UserID = u.Username, u.Role, u.ID
	}
	p.Encrypted = r.URL.Query().Get("legacy") != "1"
	return p
}

func messageInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, messagingSvc.ErrChatInvalid
	}
	return n, nil
}
func messageTarget(v url.Values) (messagingSvc.ChatTarget, error) {
	t := messagingSvc.ChatTarget{Handle: v.Get("handle")}
	for key, dest := range map[string]*int{"product_id": &t.ProductID, "service_id": &t.ServiceID, "order_id": &t.OrderID, "partner_id": &t.PartnerID, "delivery_order_id": &t.DeliveryOrderID} {
		n, err := messageInt(v.Get(key))
		if err != nil {
			return t, err
		}
		*dest = n
	}
	return t, nil
}

func messageError(err error) (int, string) {
	switch {
	case errors.Is(err, messagingSvc.ErrChatEncryptionRequired):
		return http.StatusConflict, "Open encrypted messages to send. Plaintext messaging is disabled."
	case errors.Is(err, messagingSvc.ErrChatNotFound):
		return http.StatusNotFound, "This conversation or contact is unavailable. Use the exact handle, or contact a courier from your delivery."
	case errors.Is(err, messagingSvc.ErrChatInvalid):
		return http.StatusBadRequest, "Check the contact and enter a message of 1–4,000 characters."
	case errors.Is(err, messagingSvc.ErrChatClosed):
		return http.StatusConflict, "This conversation is waiting for acceptance or is closed."
	case errors.Is(err, messagingSvc.ErrChatBlocked):
		return http.StatusForbidden, "Messaging is unavailable for this contact."
	case errors.Is(err, messagingSvc.ErrChatLimited):
		return http.StatusTooManyRequests, "You've reached a messaging limit. Please try again later."
	case errors.Is(err, messagingSvc.ErrChatConflict):
		return http.StatusConflict, "That message was already submitted. Refresh the conversation before sending another."
	default:
		return http.StatusInternalServerError, "Messages are temporarily unavailable. Please try again."
	}
}
func messageFailure(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := messageError(err)
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
	}
	app.RenderErrorPage(w, r, status, http.StatusText(status), msg)
}

func messageInbox(a *app.App, workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		before, err := messageInt(r.URL.Query().Get("before"))
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		p := messagesPage(a, r, workspace, "inbox")
		p.Inbox, err = a.Messaging.Inbox(r.Context(), messagingActor(r), before)
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		p.Flash = popFlash(w, r)
		a.Render(w, a.Templates.Messages, p)
	}
}

func messageCompose(a *app.App, workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		p := messagesPage(a, r, workspace, "compose")
		t, err := messageTarget(r.URL.Query())
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		p.Target = t
		status := http.StatusOK
		if t.Handle != "" || t.ServiceID > 0 || t.ProductID > 0 || t.OrderID > 0 || t.DeliveryOrderID > 0 || t.PartnerID > 0 {
			p.Contacts, err = a.Messaging.Contacts(r.Context(), messagingActor(r), t)
			if err != nil {
				status, p.Error = messageError(err)
			}
		}
		p.Nonce, err = messagingSvc.Nonce()
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		a.RenderStatus(w, a.Templates.Messages, status, p)
	}
}

func messageStart(a *app.App, workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		if err := r.ParseForm(); err != nil {
			messageFailure(w, r, messagingSvc.ErrChatInvalid)
			return
		}
		t, err := messageTarget(r.PostForm)
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		id, err := a.Messaging.Start(r.Context(), messagingActor(r), t, r.PostForm.Get("body"), r.PostForm.Get("nonce"))
		if err == nil {
			http.Redirect(w, r, "/messages/"+strconv.Itoa(id), http.StatusSeeOther)
			return
		}
		status, msg := messageError(err)
		if id > 0 {
			redirectWithFlash(w, r, "/messages/"+strconv.Itoa(id), "danger", msg)
			return
		}
		p := messagesPage(a, r, workspace, "compose")
		p.Target, p.Error, p.Nonce = t, msg, r.PostForm.Get("nonce")
		if len(r.PostForm.Get("body")) <= 16000 {
			p.Body = r.PostForm.Get("body")
		}
		p.Contacts, err = a.Messaging.Contacts(r.Context(), messagingActor(r), t)
		if err != nil {
			status, p.Error = messageError(err)
		}
		if p.Nonce == "" {
			p.Nonce, err = messagingSvc.Nonce()
			if err != nil {
				messageFailure(w, r, err)
				return
			}
		}
		a.RenderStatus(w, a.Templates.Messages, status, p)
	}
}

func loadMessagePage(a *app.App, r *http.Request, workspace string) (MessagesPageData, error) {
	p := messagesPage(a, r, workspace, "thread")
	id, err := messageInt(r.PathValue("id"))
	if err != nil {
		return p, err
	}
	before, err := messageInt(r.URL.Query().Get("before"))
	if err != nil {
		return p, err
	}
	p.Thread, err = a.Messaging.Thread(r.Context(), messagingActor(r), id, before, 0)
	if err != nil {
		return p, err
	}
	p.OlderPage = before > 0
	if n := len(p.Thread.Messages); n > 0 {
		p.LastMessageID = p.Thread.Messages[n-1].ID
	}
	p.Nonce, err = messagingSvc.Nonce()
	return p, err
}
func messageThread(a *app.App, workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		p, err := loadMessagePage(a, r, workspace)
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		p.Flash = popFlash(w, r)
		a.Render(w, a.Templates.Messages, p)
	}
}
func messageSend(a *app.App, workspace string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		if err := r.ParseForm(); err != nil {
			messageFailure(w, r, messagingSvc.ErrChatInvalid)
			return
		}
		id, err := messageInt(r.PathValue("id"))
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		_, err = a.Messaging.Send(r.Context(), messagingActor(r), id, r.PostForm.Get("body"), r.PostForm.Get("nonce"))
		if err == nil {
			http.Redirect(w, r, "/messages/"+strconv.Itoa(id), http.StatusSeeOther)
			return
		}
		status, msg := messageError(err)
		p, loadErr := loadMessagePage(a, r, workspace)
		if loadErr != nil {
			messageFailure(w, r, loadErr)
			return
		}
		p.Error, p.Nonce = msg, r.PostForm.Get("nonce")
		if len(r.PostForm.Get("body")) <= 16000 {
			p.Body = r.PostForm.Get("body")
		}
		a.RenderStatus(w, a.Templates.Messages, status, p)
	}
}
func messageAction(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		if err := r.ParseForm(); err != nil {
			messageFailure(w, r, messagingSvc.ErrChatInvalid)
			return
		}
		id, err := messageInt(r.PathValue("id"))
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		messageID, err := messageInt(r.PostForm.Get("message_id"))
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		action := r.PostForm.Get("action")
		err = a.Messaging.Action(r.Context(), messagingActor(r), id, action, messageID, r.PostForm.Get("reason"))
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		notices := map[string]string{"accept": "Message request accepted.", "decline": "Message request declined.", "block": "Contact blocked across conversations.", "unblock": "Your block was removed.", "report": "Reported message sent to the moderation team."}
		if msg := notices[action]; msg != "" {
			redirectWithFlash(w, r, "/messages/"+strconv.Itoa(id), "success", msg)
			return
		}
		http.Redirect(w, r, "/messages/"+strconv.Itoa(id), http.StatusSeeOther)
	}
}

type MessageReportsPageData struct {
	Title, Role, CSRFToken string
	Reports                []messagingSvc.ChatReport
	Flash                  Flash
}

func messageReports(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			id, err := messageInt(r.FormValue("report_id"))
			if err == nil {
				if r.FormValue("encrypted") == "true" {
					id = -id
				}
				err = a.Messaging.ResolveReport(r.Context(), messagingActor(r), id)
			}
			if err != nil {
				messageFailure(w, r, err)
				return
			}
			redirectWithFlash(w, r, "/message-reports", "success", "Report marked reviewed.")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead, http.MethodPost)
			return
		}
		reports, err := a.Messaging.Reports(r.Context(), messagingActor(r))
		if err != nil {
			messageFailure(w, r, err)
			return
		}
		a.Render(w, a.Templates.MessageReports, MessageReportsPageData{Title: "Message reports", Role: domain.RoleAdmin, CSRFToken: a.Auth.CSRFToken(r), Reports: reports, Flash: popFlash(w, r)})
	}
}
