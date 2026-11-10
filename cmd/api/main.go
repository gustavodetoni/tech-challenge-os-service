package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rabbitmq/amqp091-go"
)

type ServiceOrder struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	ClientID    string    `json:"client_id"`
	VehicleID   string    `json:"vehicle_id"`
	AmountCents int64     `json:"amount_cents"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}
type Event struct {
	ID         string    `json:"event_id"`
	Type       string    `json:"type"`
	SagaID     string    `json:"saga_id"`
	OrderID    string    `json:"order_id"`
	OccurredAt time.Time `json:"occurred_at"`
	Data       any       `json:"data,omitempty"`
}
type App struct {
	db        *sql.DB
	publisher *Publisher
	logger    *slog.Logger
}
type Publisher struct {
	conn     *amqp091.Connection
	channel  *amqp091.Channel
	exchange string
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	db, err := sql.Open("pgx", env("DATABASE_URL", "postgres://tech_challenge:tech_challenge@localhost:5432/os_service?sslmode=disable"))
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		panic(err)
	}
	if err = migrate(ctx, db); err != nil {
		panic(err)
	}
	app := &App{db: db, publisher: newPublisher(logger), logger: logger}
	if app.publisher != nil {
		go app.consumeEvents()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", app.health)
	mux.HandleFunc("POST /service-orders", app.create)
	mux.HandleFunc("GET /service-orders/{id}", app.get)
	mux.HandleFunc("POST /service-orders/{id}/cancel", app.cancel)
	logger.Info("os service started", "port", env("PORT", "8081"))
	if err := http.ListenAndServe(":"+env("PORT", "8081"), withCorrelation(mux)); err != nil {
		logger.Error("server stopped", "error", err)
	}
}

func migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS service_orders (id uuid PRIMARY KEY, code text NOT NULL UNIQUE, client_id uuid NOT NULL, vehicle_id uuid NOT NULL, status text NOT NULL, saga_id uuid NOT NULL, created_at timestamptz NOT NULL); CREATE TABLE IF NOT EXISTS outbox (id uuid PRIMARY KEY, event_type text NOT NULL, aggregate_id uuid NOT NULL, payload jsonb NOT NULL, published_at timestamptz); CREATE INDEX IF NOT EXISTS idx_os_outbox_pending ON outbox(published_at) WHERE published_at IS NULL`)
	return err
}
func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, 200, map[string]string{"status": "ok", "service": "os-service"})
}
func (a *App) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ClientID    string `json:"client_id"`
		VehicleID   string `json:"vehicle_id"`
		AmountCents int64  `json:"amount_cents"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.ClientID == "" || in.VehicleID == "" || in.AmountCents <= 0 {
		jsonResponse(w, 400, map[string]string{"error": "client_id, vehicle_id and positive amount_cents are required"})
		return
	}
	id, saga := uuid.New(), uuid.New()
	code := "OS-" + strings.ToUpper(uuid.NewString()[:8])
	now := time.Now().UTC()
	order := ServiceOrder{ID: id.String(), Code: code, ClientID: in.ClientID, VehicleID: in.VehicleID, AmountCents: in.AmountCents, Status: "RECEIVED", CreatedAt: now}
	event := Event{ID: uuid.NewString(), Type: "ServiceOrderCreated", SagaID: saga.String(), OrderID: id.String(), OccurredAt: now, Data: order}
	payload, _ := json.Marshal(event)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO service_orders(id,code,client_id,vehicle_id,status,saga_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, code, in.ClientID, in.VehicleID, order.Status, saga, now)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO outbox(id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4)`, uuid.New(), event.Type, id, payload)
	}
	if err != nil {
		tx.Rollback()
		jsonResponse(w, 500, map[string]string{"error": "could not create service order"})
		return
	}
	if err = tx.Commit(); err != nil {
		jsonResponse(w, 500, map[string]string{"error": "could not commit service order"})
		return
	}
	a.publish(event)
	jsonResponse(w, 201, order)
}
func (a *App) get(w http.ResponseWriter, r *http.Request) {
	var o ServiceOrder
	err := a.db.QueryRowContext(r.Context(), `SELECT id,code,client_id,vehicle_id,status,created_at FROM service_orders WHERE id=$1`, r.PathValue("id")).Scan(&o.ID, &o.Code, &o.ClientID, &o.VehicleID, &o.Status, &o.CreatedAt)
	if err != nil {
		jsonResponse(w, 404, map[string]string{"error": "service order not found"})
		return
	}
	jsonResponse(w, 200, o)
}
func (a *App) cancel(w http.ResponseWriter, r *http.Request) {
	result, err := a.db.ExecContext(r.Context(), `UPDATE service_orders SET status='CANCELED' WHERE id=$1 AND status NOT IN ('FINISHED','DELIVERED')`, r.PathValue("id"))
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		jsonResponse(w, 409, map[string]string{"error": "order cannot be canceled"})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "CANCELED"})
}
func (a *App) publish(e Event) {
	if a.publisher == nil {
		a.logger.Info("event queued", "event_type", e.Type, "order_id", e.OrderID)
		return
	}
	body, _ := json.Marshal(e)
	_ = a.publisher.channel.PublishWithContext(context.Background(), a.publisher.exchange, "order."+e.Type, false, false, amqp091.Publishing{ContentType: "application/json", Body: body, MessageId: e.ID, Type: e.Type, CorrelationId: e.SagaID})
}
func (a *App) consumeEvents() {
	ch, err := a.publisher.conn.Channel()
	if err != nil {
		return
	}
	defer ch.Close()
	_ = ch.QueueBind("os-service-saga", "billing.PaymentConfirmed", "tech-challenge.events", false, nil)
	_ = ch.QueueBind("os-service-saga", "execution.ServiceOrderExecutionCompleted", "tech-challenge.events", false, nil)
	msgs, err := ch.Consume("os-service-saga", "os-service", false, false, false, false, nil)
	if err != nil {
		return
	}
	for msg := range msgs {
		var e Event
		if json.Unmarshal(msg.Body, &e) == nil {
			status := "IN_PROGRESS"
			if e.Type == "ServiceOrderExecutionCompleted" {
				status = "FINISHED"
			}
			_, _ = a.db.Exec(`UPDATE service_orders SET status=$1 WHERE id=$2`, status, e.OrderID)
		}
		_ = msg.Ack(false)
	}
}
func newPublisher(logger *slog.Logger) *Publisher {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		return nil
	}
	c, e := amqp091.Dial(url)
	if e != nil {
		logger.Error("rabbitmq unavailable", "error", e)
		return nil
	}
	ch, e := c.Channel()
	if e != nil {
		return nil
	}
	_ = ch.ExchangeDeclare("tech-challenge.events", "topic", true, false, false, false, nil)
	return &Publisher{conn: c, channel: ch, exchange: "tech-challenge.events"}
}
func withCorrelation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Correlation-ID", id)
		next.ServeHTTP(w, r)
	})
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
