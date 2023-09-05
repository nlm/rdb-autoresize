# RDB Autoresize

Automatic resizer for RDB volumes.

This tool helps you keep the size of your Scaleway Database Instances volumes adapted to the
amount of data you store in it.

## How to use

What you need:

- A Scaleway API Key (access key + secret key)
- A Scaleway Database instance

### Using docker compose

```bash
export SCW_ACCESS_KEY="a-scaleway-access-key"
export SCW_SECRET_KEY="a-scaleway-secret-key"
export SCW_RDB_REGION="the-region-your-rdb-instance-is-deployed-on"
export SCW_RDB_INSTANCE_ID="your-rdb-instance-id"

docker-compose up -d
```

### Manually

```bash
go build

export SCW_ACCESS_KEY="a-scaleway-access-key"
export SCW_SECRET_KEY="a-scaleway-secret-key"
export SCW_RDB_REGION="the-region-your-rdb-instance-is-deployed-on"
export SCW_RDB_INSTANCE_ID="your-rdb-instance-id"

./rdb-autoresize
```
