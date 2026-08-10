package queue

import (
	"github.com/rabbitmq/amqp091-go"
	"github.com/wagslane/go-rabbitmq"
)

// NewConn abre una conexión a RabbitMQ reutilizando la misma configuración
// del enqueuer.
func NewConn() (*rabbitmq.Conn, error) {
	return rabbitmq.NewConn(rabbitURL())
}

// NewConsumer declara la topología que el worker necesita para consumir: la
// cola durable y su binding al exchange. Ambas declaraciones son idempotentes,
// por lo que pueden ejecutarse cada vez que el worker arranca. El exchange
// también se declara de forma idempotente para que el worker no falle si
// arranca antes que la API.
func NewConsumer(conn *rabbitmq.Conn) (*rabbitmq.Consumer, error) {
	return rabbitmq.NewConsumer(
		conn,
		QueueName,
		rabbitmq.WithConsumerOptionsQueueDurable,
		rabbitmq.WithConsumerOptionsExchangeName(ExchangeName),
		rabbitmq.WithConsumerOptionsExchangeDurable,
		rabbitmq.WithConsumerOptionsExchangeKind(amqp091.ExchangeDirect),
		rabbitmq.WithConsumerOptionsExchangeDeclare,
		rabbitmq.WithConsumerOptionsRoutingKey(RoutingKey),
	)
}
