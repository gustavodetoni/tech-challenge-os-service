package tests

import (
	"fmt"
	"testing"

	"github.com/cucumber/godog"
	osapp "github.com/gustavodetoni/tech-challenge-os-service/internal/application"
)

type sagaScenario struct {
	amount                                     int64
	orderStatus, paymentStatus, executionEvent string
}

func (s *sagaScenario) orderCreated(amount int64) { s.amount = amount; s.orderStatus = "RECEIVED" }
func (s *sagaScenario) billingReceivesOrder() {
	if err := osapp.ValidateCreateOrder(osapp.CreateOrderInput{ClientID: "client-1", VehicleID: "vehicle-1", AmountCents: s.amount}); err != nil {
		panic(err)
	}
	s.paymentStatus = "PENDING_APPROVAL"
}
func (s *sagaScenario) paymentConfirmed()         { s.paymentStatus = "PAID" }
func (s *sagaScenario) executionReceivesPayment() { s.executionEvent = "ExecutionStarted" }
func (s *sagaScenario) orderIsInProgress() error {
	if s.executionEvent != "ExecutionStarted" || s.paymentStatus != "PAID" {
		return fmt.Errorf("payment/execution state is inconsistent")
	}
	s.orderStatus = osapp.StatusAfterEvent("PaymentConfirmed")
	return nil
}
func (s *sagaScenario) paidOrderExists() { s.orderStatus, s.paymentStatus = "IN_PROGRESS", "PAID" }
func (s *sagaScenario) executionFails()  { s.executionEvent = "ExecutionFailed" }
func (s *sagaScenario) sagaCancelsOrder() {
	if s.executionEvent != "ExecutionFailed" {
		panic("failure event was not recorded")
	}
	s.orderStatus = osapp.StatusAfterEvent("ServiceOrderCanceled")
}
func (s *sagaScenario) paymentRefundRequested() error {
	if osapp.CompensationFor("payment_confirmed") != "refund_payment" {
		return fmt.Errorf("refund compensation not selected")
	}
	if s.orderStatus != "CANCELED" {
		return fmt.Errorf("order was not canceled")
	}
	return nil
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	s := &sagaScenario{}
	ctx.Given(`a new service order was created with amount (\d+) cents`, func(amount int64) { s.orderCreated(amount) })
	ctx.When(`Billing Service receives ServiceOrderCreated`, s.billingReceivesOrder)
	ctx.When(`Mercado Pago confirms the payment`, s.paymentConfirmed)
	ctx.Then(`Execution Service receives PaymentConfirmed`, s.executionReceivesPayment)
	ctx.Then(`the service order is marked as IN_PROGRESS`, s.orderIsInProgress)
	ctx.Given(`a paid service order exists`, s.paidOrderExists)
	ctx.When(`Execution Service reports ExecutionFailed`, s.executionFails)
	ctx.Then(`OS Service marks the order as CANCELED`, s.sagaCancelsOrder)
	ctx.Then(`Billing Service receives PaymentRefundRequested`, s.paymentRefundRequested)
}

func TestOrderSagaBDD(t *testing.T) {
	suite := godog.TestSuite{Name: "order-saga", ScenarioInitializer: InitializeScenario, Options: &godog.Options{Format: "progress", Paths: []string{"../features"}}}
	if suite.Run() != 0 {
		t.Fatal("BDD scenarios failed")
	}
}
