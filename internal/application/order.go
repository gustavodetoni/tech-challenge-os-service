package application

import "fmt"

type CreateOrderInput struct {
	ClientID    string
	VehicleID   string
	AmountCents int64
}

func ValidateCreateOrder(in CreateOrderInput) error {
	if in.ClientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if in.VehicleID == "" {
		return fmt.Errorf("vehicle_id is required")
	}
	if in.AmountCents <= 0 {
		return fmt.Errorf("amount_cents must be positive")
	}
	return nil
}

func CompensationFor(step string) string {
	switch step {
	case "order_created", "quote_created":
		return "cancel_service_order"
	case "payment_confirmed":
		return "refund_payment"
	case "execution_started":
		return "cancel_execution"
	default:
		return "record_saga_failure"
	}
}

func StatusAfterEvent(eventType string) string {
	switch eventType {
	case "PaymentConfirmed":
		return "IN_PROGRESS"
	case "ServiceOrderExecutionCompleted":
		return "FINISHED"
	case "ServiceOrderCanceled":
		return "CANCELED"
	case "ExecutionFailed":
		return "CANCELED"
	default:
		return "UNCHANGED"
	}
}
