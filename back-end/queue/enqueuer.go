package queue

import (
	"encoding/json"
	"fmt"

	"back-end/utils"

	"github.com/rabbitmq/amqp091-go"
	"github.com/wagslane/go-rabbitmq"
)

const (
	// ExchangeName es el exchange direct durable al que publica el enqueuer.
	// El worker lo usa para declarar su cola y binding.
	ExchangeName = "conversions"
	// QueueName es la cola durable de la que consume el worker.
	QueueName = "conversions"
	// RoutingKey enlaza la cola de conversiones al exchange.
	RoutingKey = "conversion"
)

// ConversionJob es el contrato del mensaje publicado en la cola de conversiones.
// Solo transporta el ID de la conversión; el worker consulta los metadatos y
// obtiene el archivo en S3 a partir de ese ID.
type ConversionJob struct {
	ConversionID uint `json:"conversion_id"`
}

type Enqueuer struct {
	conn      *rabbitmq.Conn
	publisher *rabbitmq.Publisher
}

func NewEnqueuer() (*Enqueuer, error) {
	conn, err := rabbitmq.NewConn(rabbitURL())
	if err != nil {
		return nil, err
	}

	// El exchange se declara de forma idempotente al publicar. La cola y el
	// binding los declara el worker (consumidor) al arrancar.
	publisher, err := rabbitmq.NewPublisher(
		conn,
		rabbitmq.WithPublisherOptionsExchangeName(ExchangeName),
		rabbitmq.WithPublisherOptionsExchangeDeclare,
		rabbitmq.WithPublisherOptionsExchangeDurable,
		rabbitmq.WithPublisherOptionsExchangeKind(amqp091.ExchangeDirect),
	)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &Enqueuer{conn: conn, publisher: publisher}, nil
}

func (e *Enqueuer) EnqueueConversion(conversionID uint) error {
	/*
		EnqueueConversion publica un mensaje en la cola de conversiones con el ID de la conversión que debe procesarse.
		El worker consumirá este mensaje, consultará los metadatos de la conversión en la base de datos y obtendrá el archivo correspondiente desde S3 para procesarlo.
	*/

	body, err := json.Marshal(ConversionJob{ConversionID: conversionID})
	if err != nil {
		return err
	}
	return e.publisher.Publish(
		body,
		[]string{RoutingKey},
		rabbitmq.WithPublishOptionsExchange(ExchangeName),
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsPersistentDelivery,
	)
}

func (e *Enqueuer) Close() error {
	e.publisher.Close()
	return e.conn.Close()
}

func rabbitURL() string {
	/*
		rabbitURL construye la URL de conexión a RabbitMQ a partir de las variables de entorno.
		Si no se encuentran las variables, se usan valores por defecto.
	*/
	host := utils.GetEnv("RABBITMQ_HOST", "localhost")
	port := utils.GetEnv("RABBITMQ_PORT", "5672")
	user := utils.GetEnv("RABBITMQ_DEFAULT_USER", "guest")
	pass := utils.GetEnv("RABBITMQ_DEFAULT_PASS", "guest")
	return fmt.Sprintf("amqp://%s:%s@%s:%s/", user, pass, host, port)
}
