package main
import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	kazoo "github.com/krallistic/kazoo-go"
	"github.com/Shopify/sarama"

	"strconv"

	"fmt"
)

var (
	listenAddress       = flag.String("listen-address", ":8080", "The address on which to expose the web interface and generated Prometheus metrics.")
	metricsEndpoint     = flag.String("telemetry-path", "/metrics", "Path under which to expose metrics.")
	zookeeperConnect = flag.String("zookeeper-connect", "localhost:2181",  "Zookeeper connection string")
	clusterName = flag.String("cluster-name", "kafka-cluster", "Name of the Kafka cluster used in static label")
	refreshInterval = flag.Int("refresh-interval", 15, "Seconds to sleep in between refreshes")
)

var (


	partitionOffsetDesc = prometheus.NewDesc(
		"kafka_prartion_current_offset",
		"Current Offset of a Partition",
		[]string{"topic", "partition"},
		map[string]string{"cluster": *clusterName},
	)

	consumerGroupOffset = prometheus.NewDesc(
		"kafka_consumergroup_current_offset",
		"",
		[]string{"consumergroup", "topic", "partition"},
		map[string]string{"cluster": *clusterName},
	)

	consumergroupGougeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "kafka",
		Subsystem: "consumergroup",
		Name: "current_offset",
		Help: "Current Offset of a ConsumerGroup at Topic/Partition",
		ConstLabels: map[string]string{"cluster": *clusterName},
		},
		[]string{"consumergroup", "topic", "partition"},
	)
	consumergroupLagGougeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "kafka",
		Subsystem: "consumergroup",
		Name: "lag",
		Help: "Current Approximate Lag of a ConsumerGroup at Topic/Partition",
		ConstLabels: map[string]string{"cluster": *clusterName},
	},
		[]string{"consumergroup", "topic", "partition"},
	)

	brokerGougeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "kafka",
		Subsystem: "broker",
		Name: "current_offset",
		Help: "Current Offset of a Broker at Topic/Partition",
		ConstLabels: map[string]string{"cluster": *clusterName},
		},
		[]string{"topic", "partition"},
	)




)
var zookeeperClient *kazoo.Kazoo
var brokerClient sarama.Client

func init() {
	// Register the summary and the histogram with Prometheus's default registry.
	prometheus.MustRegister(consumergroupGougeVec)
	prometheus.MustRegister(brokerGougeVec)
	prometheus.MustRegister(consumergroupLagGougeVec)

}

func updateOffsets() {
	startTime := time.Now()
	fmt.Println("Updating Stats, Time: ", time.Now())
	groups, err := zookeeperClient.Consumergroups()
	if err != nil {
		panic(err)
	}

	for _, group := range groups {
		offsets, _ := group.FetchAllOffsets()
		for topicName, partitions :=  range offsets {
			for partition, offset := range partitions{
				//TODO dont recreate Labels everytime
				consumerGroupLabels := map[string]string{"consumergroup": group.Name, "topic": topicName, "partition": strconv.Itoa(int(partition))}
				consumergroupGougeVec.With(consumerGroupLabels).Set(float64(offset))
				brokerOffset, err := brokerClient.GetOffset(topicName, partition, sarama.OffsetNewest)
				if err != nil {
					//TODO
					fmt.Println(err)
				}
				brokerLabels := map[string]string{"topic": topicName, "partition": strconv.Itoa(int(partition))}
				brokerGougeVec.With(brokerLabels).Set(float64(brokerOffset))

				consumerGroupLag := brokerOffset - offset
				consumergroupLagGougeVec.With(consumerGroupLabels).Set(float64(consumerGroupLag))
				
			}
		}
	}
	fmt.Println("Done Update: ", time.Until(startTime))


}

func updateBrokerOffsets() {

}

func main() {
	flag.Parse()


	var err error
	zookeeperClient, err = kazoo.NewKazooFromConnectionString(*zookeeperConnect, nil)
	if err != nil {
		panic(err)
	}

	brokers, err := zookeeperClient.BrokerList()
	if err != nil {
		panic(err)
	}

	config := sarama.NewConfig()
	brokerClient, err = sarama.NewClient(brokers, config)



	// Periodically record some sample latencies for the three services.
	go func() {
		for {
			updateOffsets()
			time.Sleep(time.Duration(time.Duration(*refreshInterval) * time.Second))
		}
	}()



	// Expose the registered metrics via HTTP.
	http.Handle(*metricsEndpoint, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}