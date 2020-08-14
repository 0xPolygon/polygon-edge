
# Polygon SDK


Polygon SDK is a modular and extensible framework for building Ethereum-compatible blockchain networks. 

This repository is the first implementation of Polygon SDK, written in Golang. Other implementations, written in other programming languages might be introduced in the future. If you would like to contribute to this or any future implementation, please reach out to [Polygon team](mailto:contact@polygon.technology).

To find out more about Polygon, visit the [official website](https://polygon.technology/).

WARNING: This is a work in progress so architectural changes may happen in the future. The code has not been audited yet, so please contact [Polygon team](mailto:contact@polygon.technology) if you would like to use it in production.

## Commands

### Agent

Starts the Ethereum client for the mainnet:

```
$ go run main.go agent [--config ./config.json]
```

The configuration file can be specified either in HCL or JSON format:

```
{
    "data-dir": "/tmp/data-dir"
}
```

Some attributes can be also set from the command line:

```
$ go run main.go agent --config ./config.json --data-dir /tmp/local --port 30304 --log-level TRACE
```

The values from the CLI have preference over the ones in the configuration file.

### Dev

Start a development chain with instant sealing:

```
$ go run main.go dev
```

### Genesis

Generates a test genesis file:

```
$ go run main.go genesis
```
