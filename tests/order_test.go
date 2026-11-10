package tests

import (
	"testing"

	"github.com/gustavodetoni/tech-challenge-os-service/internal/application"
)

func TestValidateCreateOrder(t *testing.T) {
	cases := []struct {
		name    string
		input   application.CreateOrderInput
		wantErr bool
	}{{"valid", application.CreateOrderInput{ClientID: "client", VehicleID: "vehicle", AmountCents: 15000}, false}, {"missing client", application.CreateOrderInput{VehicleID: "vehicle", AmountCents: 1}, true}, {"missing vehicle", application.CreateOrderInput{ClientID: "client", AmountCents: 1}, true}, {"zero amount", application.CreateOrderInput{ClientID: "client", VehicleID: "vehicle"}, true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if (application.ValidateCreateOrder(tc.input) != nil) != tc.wantErr {
				t.Fatalf("unexpected validation result")
			}
		})
	}
}

func TestSagaCompensations(t *testing.T) {
	cases := map[string]string{"order_created": "cancel_service_order", "quote_created": "cancel_service_order", "payment_confirmed": "refund_payment", "execution_started": "cancel_execution", "unknown": "record_saga_failure"}
	for step, want := range cases {
		if got := application.CompensationFor(step); got != want {
			t.Errorf("%s: got %s want %s", step, got, want)
		}
	}
}

func TestStatusAfterEvent(t *testing.T) {
	cases := map[string]string{"PaymentConfirmed": "IN_PROGRESS", "ServiceOrderExecutionCompleted": "FINISHED", "ServiceOrderCanceled": "CANCELED", "Other": "UNCHANGED"}
	for event, want := range cases {
		if got := application.StatusAfterEvent(event); got != want {
			t.Errorf("%s: got %s want %s", event, got, want)
		}
	}
}
