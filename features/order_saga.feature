Feature: Service order distributed saga

  Scenario: Paid service order is sent to execution
    Given a new service order was created with amount 15000 cents
    When Billing Service receives ServiceOrderCreated
    And Mercado Pago confirms the payment
    Then Execution Service receives PaymentConfirmed
    And the service order is marked as IN_PROGRESS

  Scenario: Execution failure is compensated
    Given a paid service order exists
    When Execution Service cannot start the repair
    Then the saga records the failure
    And the service order can be canceled through its compensation command
