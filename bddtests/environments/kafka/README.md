# Apache Kafka Docker Image
This image can be used to start Apache Kafka in a Docker container.

Use the provided [`docker-compose.yml`](docker-compose.yml) file as a starting point.

## Usage
#### Start
```console
$ docker-compose up -d
Creating kafka_zookeeper_1
Creating kafka_kafka_1
$
```
#### Scale
```console
$ docker-compose scale kafka=3
Creating and starting kafka_kafka_2 ... done
Creating and starting kafka_kafka_3 ... done
$
```
#### Stop
```console
$ docker-compose stop
Stopping kafka_kafka_3 ... done
Stopping kafka_kafka_2 ... done
Stopping kafka_kafka_1 ... done
Stopping kafka_zookeeper_1 ... done
$
```
#### Cleanup
Use `docker-compose down` instead of `docker-compose rm` when cleaning up, so that the network is purged in addition to the containers.
```console
$ docker-compose down
Stopping kafka_kafka_3 ... done
Stopping kafka_kafka_2 ... done
Stopping kafka_kafka_1 ... done
Stopping kafka_zookeeper_1 ... done
Removing kafka_kafka_3 ... done
Removing kafka_kafka_2 ... done
Removing kafka_kafka_1 ... done
Removing kafka_zookeeper_1 ... done
Removing network kafka_default
$
```
## Configuration
Edit the [`docker-compose.yml`](docker-compose.yml) file to configure.
### server.properties
To configure a Kafka server property, add it to the environment section of the Kafka service. Kafka properties map to environment variables as follows:
1. Replace dots with underscores.
2. Change to upper case.
3. Prefix with `KAFKA_`

For example, `default.replication.factor` becomes `KAFKA_DEFAULT_REPLICATION_FACTOR`.
