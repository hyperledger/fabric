# order-n-kafka-n
A scalable kafka orderer environment.
## Starting

While you can start the environment by simply executing `docker-compose -d up`, since the list of kafka brokers is computed dynamically, it is recommended that you start the kafka nodes first, and then start the orderer nodes.

For example, to start an environment with 3 orderer shims and 5 kafka brokers, issue the following commands:

```bash
$ docker-compose up -d zookeeper
$ docker-compose up -d kafka
$ docker-compose scale kafka=5
$ docker-compose up -d orderer
$ docker-compose scale orderer=3
```

## Stopping

While you can stop the environment by simply executing `docker-compose stop`, docker-compose does not enforce a reverse-dependency order on shutdown. For a cleaner shutdown, stop the individual service types independently in the following order:

```bash
$ docker-compose stop orderer
$ docker-compose stop kafka
$ docker-compose stop
```
