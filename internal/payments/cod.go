package payments

import "context"

// CashOnDelivery records an order whose payment is collected on delivery.
type CashOnDelivery struct{}

func (CashOnDelivery) Descriptor() Descriptor {
	return Descriptor{
		ID:      "cash_on_delivery",
		Name:    "Cash on delivery",
		Summary: "Customers pay the courier. No online payment.",
		Online:  false,
	}
}

func (CashOnDelivery) Start(context.Context, Config, StartRequest) (StartResult, error) {
	return StartResult{}, ErrUnsupported
}

func (CashOnDelivery) Verify(context.Context, Config, VerifyRequest) (Verification, error) {
	return Verification{Status: StatusPending}, nil
}
