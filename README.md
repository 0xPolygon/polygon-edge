# Minimal

[![Join the chat at https://gitter.im/umbracle/minimal](https://badges.gitter.im/umbracle/minimal.svg)](https://gitter.im/umbracle/minimal?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

Modular implementation of different stacks of the Ethereum blockchain

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
$ go run main.go agent --config ./config.json --data-dir /tmp/local
```

The values from the CLI have preference over the ones in the configuration file.

### Probe

Probes and returns information of a given enode:

```
$ go run main.go probe <enode://>
```

### Genesis

Generates a test genesis file:

```
$ go run main.go genesis
```
