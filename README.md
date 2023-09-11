# RDB Autoresize

Automatic resizer for RDB volumes.

This tool helps you keep the size of your Scaleway Database Instances volumes adapted to the
amount of data you store in it.

## How to use

What you need:

- A Scaleway API Key (access key + secret key)
- A Scaleway Database instance

### Using docker compose

Export mandatory variables:

```bash
export SCW_ACCESS_KEY="a-scaleway-access-key"
export SCW_SECRET_KEY="a-scaleway-secret-key"
export SCW_RDB_REGION="the-region-your-rdb-instance-is-deployed-on"
export SCW_RDB_INSTANCE_ID="your-rdb-instance-id"
```

Optionally, you can tweak additional settings:

```bash
# the % above which triggering will happen. Defaults to 90%
export SCW_RDB_TRIGGER_PERCENTAGE=90
# the limit size of your volume, defaults to 100GB
export SCW_RDB_VOLUME_SIZE_LIMIT=100GB
```

Start the stack:

```bash
docker compose up --build
```

### Manually

```bash
go build

export SCW_ACCESS_KEY="a-scaleway-access-key"
export SCW_SECRET_KEY="a-scaleway-secret-key"
export SCW_RDB_REGION="the-region-your-rdb-instance-is-deployed-on"
export SCW_RDB_INSTANCE_ID="your-rdb-instance-id"

./rdb-autoresize -volume-size-limit 100GB
```

## Configuration

Several parameters can be tweaked via environment variables:

- `SCW_RDB_TRIGGER_PERCENTAGE`: resize will happen when disk usage is above this percentage.
- `SCW_RDB_VOLUME_SIZE_LIMIT`: resize will no happen if target size is above this.

You also have some command line options:

- `-trigger-percentage`: equivalient of `SCW_RDB_TRIGGER_PERCENTAGE`
- `-volume-size-limit`: equivalent of `SCW_RDB_VOLUME_SIZE_LIMIT`
- `-log-json`: activate json-formatted logging
- `-debug`: activate debug logging
