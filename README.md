# OS Service

Microsserviço responsável pelo ciclo de vida das ordens de serviço. Possui PostgreSQL próprio, Outbox transacional e publica `ServiceOrderCreated` em RabbitMQ.

## Papel na arquitetura

Este é o serviço dono da ordem de serviço e o coordenador da Saga. Ele não consulta bancos externos. O Billing Service e o Execution Service recebem eventos por RabbitMQ.

## Contratos

- `POST /service-orders` — cria a OS e inicia a Saga.
- `GET /service-orders/{id}` — consulta a OS.
- `POST /service-orders/{id}/cancel` — compensação da criação.
- `GET /health` — healthcheck.

Contrato OpenAPI: [docs/openapi.yaml](docs/openapi.yaml).

### Criar OS

```bash
curl -X POST http://localhost:8081/service-orders \
  -H 'Content-Type: application/json' \
  -d '{"client_id":"CLIENT_UUID","vehicle_id":"VEHICLE_UUID","amount_cents":15000}'
```

## Configuração

`PORT`, `DATABASE_URL` e `RABBITMQ_URL` (`amqp://guest:guest@rabbitmq:5672/`). O serviço nunca acessa bancos de outros microsserviços.

Na inicialização são criadas as tabelas `service_orders` e `outbox`. O evento da criação carrega `event_id`, `saga_id`, `order_id`, tipo, timestamp e dados da OS.

## Mensageria e Saga

- Exchange: `tech-challenge.events`.
- Publica `order.ServiceOrderCreated`.
- Consome `billing.PaymentConfirmed` e atualiza a OS para `IN_PROGRESS`.
- Consome `execution.ServiceOrderExecutionCompleted` e atualiza a OS para `FINISHED`.
- `POST /service-orders/{id}/cancel` é a compensação local de cancelamento.

### Estratégia escolhida

Foi adotada uma Saga orquestrada pelo OS Service. O OS Service é o dono do estado da ordem e a coordenação acontece exclusivamente por eventos RabbitMQ, sem acesso ao banco dos outros serviços.

Fluxo de sucesso:

```text
ServiceOrderCreated
→ criação do orçamento
→ PaymentConfirmed
→ início da execução
→ ServiceOrderExecutionCompleted
→ OS marcada como FINISHED
```

Fluxo de falha e compensação:

```text
ExecutionFailed
→ OS Service marca a OS como CANCELED
→ PaymentRefundRequested
→ Billing Service solicita estorno
→ orçamento marcado como REFUNDED
```

O fluxo documentado em [features/order_saga.feature](features/order_saga.feature) é executado por Godog em [tests/order_saga_bdd_test.go](tests/order_saga_bdd_test.go), cobrindo pagamento confirmado e falha de execução com solicitação de estorno.

## Execução local

```bash
make tests
make build
docker compose -f ../tech-challenge-project/docker-compose.phase4.yml up --build os-service
```

## CI/CD e observabilidade

- `ci.yml`: formatação, `go vet`, testes, cobertura e build.
- `sonar.yml`: análise SonarQube.
- `deploy.yml`: build/push Docker e rollout Kubernetes.
- Logs são emitidos em JSON e as requisições propagam `X-Correlation-ID`.

## Testes, BDD e cobertura

Os testes unitários ficam em [tests](tests). O cenário BDD Godog em [tests/order_saga_bdd_test.go](tests/order_saga_bdd_test.go), baseado em [features/order_saga.feature](features/order_saga.feature), cobre o fluxo de pagamento e a compensação após falha da execução.

```bash
make tests
```

Resultado atual:

```text
internal/application: 100%
```

O CI exige cobertura mínima de 80% do pacote de regras de negócio usando `-coverpkg=./internal/application`.
