package payment

import (
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
)

func TestSupportedMethods(t *testing.T) {
	svc := New()
	methods := svc.SupportedMethods()
	if len(methods) != 1 {
		t.Fatalf("methods length = %d, want 1", len(methods))
	}
	if methods[0] != MethodCashOnDelivery {
		t.Fatalf("first method = %q, want %q", methods[0], MethodCashOnDelivery)
	}
}

func TestBuildRecord_CashOnDeliveryPending(t *testing.T) {
	svc := New()

	record, err := svc.BuildRecord(MethodCashOnDelivery, 4250)
	if err != nil {
		t.Fatalf("BuildRecord error = %v", err)
	}
	if record.Status != db.PaymentStatusPending {
		t.Fatalf("status = %q, want %q", record.Status, db.PaymentStatusPending)
	}
	if record.ExternalReference != "" {
		t.Fatalf("external reference = %q, want empty", record.ExternalReference)
	}
}

func TestBuildRecord_RejectsUnimplementedProviders(t *testing.T) {
	svc := New()
	for _, method := range []string{MethodCardPlaceholder, MethodMobilePlaceholder} {
		t.Run(method, func(t *testing.T) {
			if svc.IsSupportedMethod(method) {
				t.Fatal("unimplemented provider advertised as supported")
			}
			if _, err := svc.BuildRecord(method, 4250); err != ErrInvalidMethod {
				t.Fatalf("error = %v, want %v", err, ErrInvalidMethod)
			}
		})
	}
}

func TestBuildRecord_InvalidMethod(t *testing.T) {
	svc := New()

	if _, err := svc.BuildRecord("wire_transfer", 20); err != ErrInvalidMethod {
		t.Fatalf("error = %v, want %v", err, ErrInvalidMethod)
	}
}
