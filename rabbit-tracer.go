// Tracer for consuming messages from rabbitMQ server for logging
package rabbit_tracer

import (
	"fmt"
	"log"
	"time"
	"github.com/streadway/amqp"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	tracerCount int = 0
	msgcounter int = 0
)

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	tag     string
	done    chan error
	thread  int
	tracerType string 
}

func init() {
	conf = GetConfig()
	//initLogRotate()
}

func initLogRotate(tracerType string) {
	logFile := ``
	if (tracerType == `SUB`) {
		logFile = conf.Sub[`client`].Logfile
	} else {		
		logFile = conf.Pub[`client`].Logfile
	}

	log.SetOutput(&lumberjack.Logger{
		Filename:   conf.Tracer[`logging`].Logpath + logFile,
		MaxSize:    conf.Tracer[`logging`].Logfilemaxsize, // megabytes
		MaxBackups: conf.Tracer[`logging`].Logfilemaxbackup,
		MaxAge:     conf.Tracer[`logging`].Logfilemaxage, // days
	})
}

func Worker(tracerType string, done chan bool) {
    tracerCount++
	if (tracerCount == 1) {
		initLogRotate(tracerType)	
	}
	
	log.Printf("Tracer %s %d Started\n", tracerType, tracerCount)

	c, err := NewConsumer(tracerType, tracerCount)
	if err != nil {
		log.Fatalf("%s", err)
	}

	if conf.Rabbit[`server`].Lifetime > 0 {
		log.Printf("running for %s", conf.Rabbit[`server`].Lifetime)
		time.Sleep(conf.Rabbit[`server`].Lifetime)
	} else {
		log.Printf("%d. Running forever:", c.thread)
		select {}
	}

	log.Printf("%d. Shutting Down", c.thread)

	if err := c.Shutdown(); err != nil {
		log.Fatalf("%d. Error during shutdown: %s", c.thread, err)
	}

    done <- true
}

func NewConsumer(tracerType string, tracerCount int) (*Consumer, error) {
	cTag := ``
	qName := ``
	rKey :=``
	if (tracerType == `SUB`) {
		qName = conf.Sub[`client`].Queue
		rKey = conf.Sub[`client`].Bindingkey
		cTag = conf.Sub[`client`].Consumertag
	} else {
		qName = conf.Pub[`client`].Queue
		rKey = conf.Pub[`client`].Bindingkey
		cTag = conf.Pub[`client`].Consumertag
	}

	c := &Consumer{
		conn:    nil,
		channel: nil,
		tag:     cTag,
		done:    make(chan error),
		thread: tracerCount,
		tracerType: tracerType,
	}

	var err error

	log.Printf("%d. Dialing %q", c.thread, conf.Rabbit[`server`].Uri)
	c.conn, err = amqp.Dial(conf.Rabbit[`server`].Uri)
	if err != nil {
		return nil, fmt.Errorf("%d. Dial: %s", c.thread, err)
	}

	go func() {
		fmt.Printf("Closing: %s", <-c.conn.NotifyClose(make(chan *amqp.Error)))
	}()

	log.Printf("%d. Got Connection, getting Channel", c.thread)
	c.channel, err = c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("%d. Channel: %s", c.thread, err)
	}

	er := c.channel.Qos(
		conf.Rabbit[`server`].Prefetchcount,
		conf.Rabbit[`server`].Prefetchsize,
		false,
	)
	if er != nil {
		fmt.Errorf("%d. Cannot set Qos: %s", c.thread, er)
	}

	log.Printf("%d. Got Channel, declaring Exchange (%q)", c.thread, conf.Rabbit[`server`].Exchange)
	if err = c.channel.ExchangeDeclare(
		conf.Rabbit[`server`].Exchange,     // name of the exchange
		conf.Rabbit[`server`].Exchangetype, // type
		true,         // durable
		false,        // delete when complete
		true,         // internal
		false,        // noWait
		nil,          // arguments
	); err != nil {
		return nil, fmt.Errorf("%d. Exchange declare: %s", c.thread, err)
	}

	log.Printf("%d. Declared Exchange, declaring Queue %q", c.thread, qName)
	queue, err := c.channel.QueueDeclare(
		qName, // name of the queue
		true,      // durable
		false,     // delete when usused
		false,     // exclusive
		false,     // noWait
		nil,       // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("%d. Queue Declare: %s", c.thread, err)
	}

	log.Printf("%d. Declared Queue (%q %d messages, %d consumers), binding to Exchange (key %q)",
		c.thread,
		queue.Name,
		queue.Messages,
		queue.Consumers,
		rKey,
	)

	if err = c.channel.QueueBind(
		queue.Name, // name of the queue
		rKey, // bindingKey
		conf.Rabbit[`server`].Exchange, // sourceExchange
		false, // noWait
		nil, // arguments
	); err != nil {
		return nil, fmt.Errorf("%d. Queue Bind: %s", c.thread, err)
	}

	log.Printf("%d. Queue bound to Exchange, starting Consume (consumer tag %q)", c.thread, c.tag)
	deliveries, err := c.channel.Consume(
		queue.Name, // name
		c.tag,      // consumerTag,
		false,      // noAck
		false,      // exclusive
		false,      // noLocal
		false,      // noWait
		nil,        // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("%d. Queue Consume: %s", c.thread, err)
	}

	go msgHandler(deliveries, c.done, c.tracerType)

	return c, nil
}

func (c *Consumer) Shutdown() error {
	// will close() the deliveries channel
	if err := c.channel.Cancel(c.tag, true); err != nil {
		return fmt.Errorf("%d. Consumer cancel failed: %s", c.thread, err)
	}

	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("%d. AMQP connection close error: %s", c.thread, err)
	}

	defer log.Printf("%d. AMQP shutdown OK", c.thread)

	// wait for msgHandler() to exit
	return <-c.done
}

func msgHandler(deliveries <-chan amqp.Delivery, done chan error, tracerType string) {
	for d := range deliveries {
		msgcounter++
		if (tracerType == `SUB`) {
			log.Printf(
    	        "[%v]: %dByte Type:%q\nHeaders: \n%+v\nTime: %q\nExchange: %q\nRoutingKey: %q\n",
      			d.DeliveryTag, len(d.Body), d.Type,
            	d.Headers,
	        	d.Timestamp.Format(time.UnixDate),
        		d.Exchange,
            	d.RoutingKey,
        	)
        } else {
			log.Printf(
            	"[%v]: %dByte Type:%q\nHeaders: \n%+v\nTime: %q\nExchange: %q\nRoutingKey: %q\nPayLoad: %s\n\n",
      			d.DeliveryTag, len(d.Body), d.Type,
            	d.Headers,
	        	d.Timestamp.Format(time.UnixDate),
        		d.Exchange,
            	d.RoutingKey,
				string(d.Body),			
        	)
        }	
	    fmt.Printf("[%v]: %dB\n", d.DeliveryTag, len(d.Body))
		d.Ack(false)
/*
		if (msgcounter == 500) {
			d.Ack(true)
			msgcounter = 0
		}	
*/
	}

	log.Printf("handle: deliveries channel closed")
	done <- nil
}
