package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"os"
	"os/signal"

	"github.com/namsral/flag"
	"github.com/Shopify/sarama"
)

var (
	kafkaBrokerUrl     string
	kafkaVerbose       bool
	kafkaTopic         string
	kafkaConsumerGroup string
	kafkaClientId      string
	onosUsername       string
	onosPassword       string
	onosUrl            string
)

//{"timestamp":"2019-06-13T21:49:19.056Z","deviceId":"of:000000000a4001cd","portNumber":"16","authenticationState":"APPROVED"}
type AuthEvent struct {
	Timestamp           string
	DeviceId            string
	PortNumber          string
	AuthenticationState string
}

func processMessage(msg []byte) {

	var authEvent AuthEvent
	json.Unmarshal(msg, &authEvent)
	fmt.Printf("Timestamp: %s, DeviceId: %s, PortNumber: %s, AuthenticationState: %s", authEvent.Timestamp, authEvent.DeviceId, authEvent.PortNumber, authEvent.AuthenticationState)

	//curl -v --user onos:rocks -X POST http://localhost:8181/onos/olt/oltapp/of%3A000000000a4001cd/16
	if authEvent.AuthenticationState == "APPROVED" {
		log.Printf("auth-approved-pushing-onos Device Id: %s, PortNumber: %s", authEvent.DeviceId, authEvent.PortNumber)
		client := &http.Client{}
		req, err := http.NewRequest("POST", "http://"+onosUrl+"/onos/olt/oltapp/"+authEvent.DeviceId+"/"+authEvent.PortNumber, nil)
		if err != nil {
			log.Fatal(err)
		}

		req.SetBasicAuth(onosUsername, onosPassword)
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}

		defer resp.Body.Close()
		_, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("res.StatusCode: %d\n", resp.StatusCode)
			log.Fatal(err)
		}
		fmt.Printf("res.StatusCode: %d\n", resp.StatusCode)

	}
}

func kafkaLoop() {
	log.Println("starting-onos-subscriber-agent")
	brokers := strings.Split(kafkaBrokerUrl, ",")

	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true

	consumer, err := sarama.NewConsumer(brokers, config)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := consumer.Close(); err != nil {
			log.Fatalln(err)
		}
	}()

	partitionConsumer, errors := consumer.ConsumePartition(kafkaTopic, 0, sarama.OffsetNewest)
	if errors != nil {
		panic(err)
	}

	defer func() {
		if err := partitionConsumer.Close(); err != nil {
			log.Fatalln(err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	ConsumerLoop:
	for {
		select {
			case m := <-partitionConsumer.Messages():
				log.Printf("Consumed message offset %d\n", m.Offset)
				fmt.Printf("message at topic/partition/offset %v/%v/%v: %s\n", m.Topic, m.Partition, m.Offset, string(m.Value))
				processMessage(m.Value)
			case consumerError := <-partitionConsumer.Errors():
				fmt.Println("Received consumerError ", string(consumerError.Topic), string(consumerError.Partition))
				log.Fatalln(consumerError.Err)
			case <-signals:
				break ConsumerLoop
		}

	}
}

func main() {
	flag.StringVar(&kafkaBrokerUrl, "kafka-brokers", "localhost:9092", "Kafka brokers in comma separated value")
	flag.BoolVar(&kafkaVerbose, "kafka-verbose", false, "Kafka verbose logging")
	flag.StringVar(&kafkaTopic, "kafka-topic", "foo", "Kafka topic. Only one topic per worker.")
	flag.StringVar(&kafkaConsumerGroup, "kafka-consumer-group", "consumer-group", "Kafka consumer group")
	flag.StringVar(&kafkaClientId, "kafka-client-id", "my-client-id", "Kafka client id")
	flag.StringVar(&onosUsername, "onos-user", "admin", "ONOS REST API Username")
	flag.StringVar(&onosPassword, "onos-password", "admin", "ONOS REST API Password")
	flag.StringVar(&onosUrl, "onos-address", "localhost:8181", "ONOS REST API Server Address")

	flag.Parse()

	kafkaLoop()
}
