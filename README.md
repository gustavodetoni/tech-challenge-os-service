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

O fluxo documentado em [features/order_saga.feature](features/order_saga.feature) cobre aprovação/pagamento, liberação da execução e compensação. O runner BDD executável ainda é uma pendência.

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

## Cobertura e pendências

Os testes ficam em [tests](tests) e cobrem 100% do pacote `internal/application`, com limite de 80% no CI. Ainda faltam testes de integração HTTP/PostgreSQL/RabbitMQ, dispatcher real da Outbox, retries/DLQ e estado persistido completo da Saga.
