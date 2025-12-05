/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"

	dcontainer "github.com/moby/moby/api/types/container"
	dnetwork "github.com/moby/moby/api/types/network"
	dcli "github.com/moby/moby/client"
	"github.com/pkg/errors"
	"github.com/tedsuo/ifrit"
)

const KafkaDefaultImage = "confluentinc/cp-kafka:5.3.1"

// Kafka manages the execution of an instance of a dockerized CouchDB
// for tests.
type Kafka struct {
	Client        dcli.APIClient
	Image         string
	HostIP        string
	HostPort      int
	ContainerPort dnetwork.Port
	Name          string
	NetworkName   string
	StartTimeout  time.Duration

	MessageMaxBytes              int
	ReplicaFetchMaxBytes         int
	UncleanLeaderElectionEnable  bool
	DefaultReplicationFactor     int
	MinInsyncReplicas            int
	BrokerID                     int
	ZooKeeperConnect             string
	ReplicaFetchResponseMaxBytes int
	AdvertisedListeners          string

	OutputStream io.Writer

	ContainerID      string
	HostAddress      string
	ContainerAddress string
	Address          string

	mutex   sync.Mutex
	stopped bool
}

// Run runs a Kafka container. It implements the ifrit.Runner interface
func (k *Kafka) Run(sigCh <-chan os.Signal, ready chan<- struct{}) error {
	if k.Image == "" {
		k.Image = KafkaDefaultImage
	}

	if k.Name == "" {
		k.Name = DefaultNamer()
	}

	if k.HostIP == "" {
		k.HostIP = "127.0.0.1"
	}

	if k.ContainerPort.IsZero() || k.ContainerPort.Num() == 0 {
		k.ContainerPort = dnetwork.MustParsePort("9092/tcp")
	}

	if k.StartTimeout == 0 {
		k.StartTimeout = DefaultStartTimeout
	}

	if k.Client == nil {
		client, err := dcli.New(dcli.FromEnv)
		if err != nil {
			return err
		}
		k.Client = client
	}

	if k.DefaultReplicationFactor == 0 {
		k.DefaultReplicationFactor = 1
	}

	if k.MinInsyncReplicas == 0 {
		k.MinInsyncReplicas = 1
	}

	if k.ZooKeeperConnect == "" {
		k.ZooKeeperConnect = "zookeeper:2181/kafka"
	}

	if k.MessageMaxBytes == 0 {
		k.MessageMaxBytes = 1000012
	}

	if k.ReplicaFetchMaxBytes == 0 {
		k.ReplicaFetchMaxBytes = 1048576
	}

	if k.ReplicaFetchResponseMaxBytes == 0 {
		k.ReplicaFetchResponseMaxBytes = 10485760
	}

	containerOptions := dcli.ContainerCreateOptions{
		Name: k.Name,
		Config: &dcontainer.Config{
			Image: k.Image,
			Env:   k.buildEnv(),
		},
		HostConfig: &dcontainer.HostConfig{
			AutoRemove: true,
			PortBindings: map[dnetwork.Port][]dnetwork.PortBinding{
				k.ContainerPort: {{
					HostIP:   netip.MustParseAddr(k.HostIP),
					HostPort: strconv.Itoa(k.HostPort),
				}},
			},
		},
	}

	if k.NetworkName != "" {
		nw, err := k.Client.NetworkInspect(context.Background(), k.NetworkName, dcli.NetworkInspectOptions{})
		if err != nil {
			return err
		}

		containerOptions.NetworkingConfig = &dnetwork.NetworkingConfig{
			EndpointsConfig: map[string]*dnetwork.EndpointSettings{
				k.NetworkName: {
					NetworkID: nw.Network.ID,
				},
			},
		}
	}

	container, err := k.Client.ContainerCreate(context.Background(), containerOptions)
	if err != nil {
		return err
	}
	k.ContainerID = container.ID

	_, err = k.Client.ContainerStart(context.Background(), container.ID, dcli.ContainerStartOptions{})
	if err != nil {
		return err
	}
	defer k.Stop()

	res, err := k.Client.ContainerInspect(context.Background(), container.ID, dcli.ContainerInspectOptions{})
	if err != nil {
		return err
	}

	k.HostAddress = net.JoinHostPort(
		res.Container.NetworkSettings.Ports[k.ContainerPort][0].HostIP.String(),
		res.Container.NetworkSettings.Ports[k.ContainerPort][0].HostPort,
	)
	k.ContainerAddress = net.JoinHostPort(
		res.Container.NetworkSettings.Networks[res.Container.HostConfig.NetworkMode.NetworkName()].IPAddress.String(),
		strconv.Itoa(int(k.ContainerPort.Num())),
	)

	logContext, cancelLogs := context.WithCancel(context.Background())
	defer cancelLogs()
	go k.streamLogs(logContext)

	containerExit := k.wait()
	ctx, cancel := context.WithTimeout(context.Background(), k.StartTimeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return errors.Wrapf(ctx.Err(), "kafka broker in container %s did not start", k.ContainerID)
	case <-containerExit:
		return errors.New("container exited before ready")
	case <-k.ready(ctx, k.ContainerAddress):
		k.Address = k.ContainerAddress
	case <-k.ready(ctx, k.HostAddress):
		k.Address = k.HostAddress
	}

	cancel()
	close(ready)

	for {
		select {
		case err := <-containerExit:
			return err
		case <-sigCh:
			if err := k.Stop(); err != nil {
				return err
			}
		}
	}
}

func (k *Kafka) buildEnv() []string {
	env := []string{
		"KAFKA_LOG_RETENTION_MS=-1",
		// "KAFKA_AUTO_CREATE_TOPICS_ENABLE=false",
		fmt.Sprintf("KAFKA_MESSAGE_MAX_BYTES=%d", k.MessageMaxBytes),
		fmt.Sprintf("KAFKA_REPLICA_FETCH_MAX_BYTES=%d", k.ReplicaFetchMaxBytes),
		fmt.Sprintf("KAFKA_UNCLEAN_LEADER_ELECTION_ENABLE=%t", k.UncleanLeaderElectionEnable),
		fmt.Sprintf("KAFKA_DEFAULT_REPLICATION_FACTOR=%d", k.DefaultReplicationFactor),
		fmt.Sprintf("KAFKA_MIN_INSYNC_REPLICAS=%d", k.MinInsyncReplicas),
		fmt.Sprintf("KAFKA_BROKER_ID=%d", k.BrokerID),
		fmt.Sprintf("KAFKA_ZOOKEEPER_CONNECT=%s", k.ZooKeeperConnect),
		fmt.Sprintf("KAFKA_REPLICA_FETCH_RESPONSE_MAX_BYTES=%d", k.ReplicaFetchResponseMaxBytes),
		fmt.Sprintf("KAFKA_ADVERTISED_LISTENERS=EXTERNAL://localhost:%d,%s://%s:9093", k.HostPort, k.NetworkName, k.Name),
		fmt.Sprintf("KAFKA_LISTENERS=EXTERNAL://0.0.0.0:9092,%s://0.0.0.0:9093", k.NetworkName),
		fmt.Sprintf("KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=EXTERNAL:PLAINTEXT,%s:PLAINTEXT", k.NetworkName),
		fmt.Sprintf("KAFKA_INTER_BROKER_LISTENER_NAME=%s", k.NetworkName),
	}
	return env
}

func (k *Kafka) ready(ctx context.Context, addr string) <-chan struct{} {
	readyCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
			if err == nil {
				conn.Close()
				close(readyCh)
				return
			}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	return readyCh
}

func (k *Kafka) wait() <-chan error {
	exitCh := make(chan error)
	go func() {
		resWait := k.Client.ContainerWait(context.Background(), k.ContainerID, dcli.ContainerWaitOptions{})
		select {
		case res := <-resWait.Result:
			err := fmt.Errorf("kafka: process exited with %d", res.StatusCode)
			exitCh <- err
		case err := <-resWait.Error:
			exitCh <- err
		}
	}()

	return exitCh
}

func (k *Kafka) streamLogs(ctx context.Context) {
	if k.OutputStream == nil {
		return
	}

	res, err := k.Client.ContainerLogs(ctx, k.ContainerID, dcli.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		fmt.Fprintf(k.OutputStream, "log stream ended with error: %s", err)
	}
	defer res.Close()

	reader := bufio.NewReader(res)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprint(k.OutputStream, "log stream ended with cancel context")
			return
		default:
			// Loop forever dumping lines of text into the containerLogger
			// until the pipe is closed
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				k.OutputStream.Write([]byte(line))
			}
			switch err {
			case nil:
			case io.EOF:
				fmt.Fprintf(k.OutputStream, "Container %s has closed its IO channel", k.ContainerID)
				return
			default:
				fmt.Fprintf(k.OutputStream, "Error reading container output: %s", err)
				return
			}
		}
	}
}

// Start starts the Kafka container using an ifrit runner
func (k *Kafka) Start() error {
	p := ifrit.Invoke(k)

	select {
	case <-p.Ready():
		return nil
	case err := <-p.Wait():
		return err
	}
}

// Stop stops and removes the Kafka container
func (k *Kafka) Stop() error {
	k.mutex.Lock()
	if k.stopped {
		k.mutex.Unlock()
		return errors.Errorf("container %s already stopped", k.ContainerID)
	}
	k.stopped = true
	k.mutex.Unlock()

	t := 0
	_, err := k.Client.ContainerStop(context.Background(), k.ContainerID, dcli.ContainerStopOptions{
		Timeout: &t,
	})
	if err != nil {
		return err
	}

	return nil
}
