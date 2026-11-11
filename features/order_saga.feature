Feature: Service order distributed saga

  Scenario: Paid service order is sent to execution
    Given a new service order was created with amount 15000 cents
    When Billing Service receives ServiceOrderCreated
    And Mercado Pago confirms the payment
    Then Execution Service receives PaymentConfirmed
    And the service order is marked as IN_PROGRESS

  Scenario: Execution failure is compensated
    Given a paid service order exists
    When Execution Service reports ExecutionFailed
    Then OS Service marks the order as CANCELED
    And Billing Service receives PaymentRefundRequested
