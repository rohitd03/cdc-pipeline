# Alerting Guide — CDC Pipeline

This document explains how to configure and operate the Alertmanager integration for the CDC Pipeline project.

---

## Table of Contents

1. [Getting a Slack Webhook URL](#1-getting-a-slack-webhook-url)
2. [Setting Up a Free Slack Workspace for Testing](#2-setting-up-a-free-slack-workspace-for-testing)
3. [Testing Alerts by Stopping the App Container](#3-testing-alerts-by-stopping-the-app-container)
4. [Alert Reference — What Each Alert Means](#4-alert-reference)
5. [Silencing an Alert During Planned Maintenance](#5-silencing-an-alert-during-planned-maintenance)

---

## 1. Getting a Slack Webhook URL

1. Go to [https://api.slack.com/apps](https://api.slack.com/apps) and sign in.
2. Click **Create New App → From scratch**.
3. Give the app a name (e.g. `CDC Alerts`) and select your workspace.
4. In the left sidebar, click **Incoming Webhooks**.
5. Toggle **Activate Incoming Webhooks** to **On**.
6. Click **Add New Webhook to Workspace**.
7. Select the target channel (e.g. `#alerts-critical`) and click **Allow**.
8. Copy the webhook URL shown — it looks like:
   ```
   https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXX
   ```
9. Repeat steps 2–8 for a second channel (e.g. `#alerts-warnings`) to get a second webhook URL.
10. Add both URLs to your `.env` file:
    ```env
    SLACK_WEBHOOK_URL_CRITICAL=https://hooks.slack.com/services/...
    SLACK_WEBHOOK_URL_WARNINGS=https://hooks.slack.com/services/...
    ```

---

## 2. Setting Up a Free Slack Workspace for Testing

1. Go to [https://slack.com/get-started](https://slack.com/get-started) and click **Create a new workspace**.
2. Enter your email and follow the prompts to create a free workspace.
3. Create two channels: `#alerts-critical` and `#alerts-warnings`.
4. Follow the steps in [Section 1](#1-getting-a-slack-webhook-url) to create webhook URLs pointing to each channel.
5. Start the stack with `make setup` — Alertmanager will use the webhook URLs from your `.env` file.

> **Tip:** The free Slack plan is sufficient for testing. Webhook integrations are supported on all plans.

---

## 3. Testing Alerts by Deliberately Stopping the App Container

The fastest way to trigger a real alert is to stop the application container, which will cause the `ServiceDown` alert to fire within 1 minute.

```bash
# Stop the app container
docker stop cdc-app

# Watch Alertmanager — alert should appear within 1–2 minutes
make check-alertmanager

# Check Prometheus to see the alert in pending/firing state
open http://localhost:9090/alerts

# Restore the container
docker start cdc-app
```

You can also test Kafka lag alerts by pausing the consumer:

```bash
# Pause the app (stops consuming without killing the container)
docker pause cdc-app

# Produce messages to the topic to build up lag, then unpause
docker unpause cdc-app
```

To test the Alertmanager API directly (send a synthetic alert):

```bash
curl -X POST http://localhost:9093/api/v2/alerts \
  -H 'Content-Type: application/json' \
  -d '[{
    "labels": {"alertname":"TestAlert","severity":"warning","job":"cdc-pipeline"},
    "annotations": {"summary":"Test alert","description":"This is a synthetic test alert."}
  }]'
```

---

## 4. Alert Reference

### Service Alerts

| Alert | Condition | Severity | Action |
|---|---|---|---|
| `ServiceDown` | `up{job="cdc-pipeline"} == 0` for 1m | Critical | Restart the app container (`docker start cdc-app`). Check app logs (`make logs`). |
| `HighErrorRate` | HTTP 5xx rate > 5% over 5m | Critical | Check application logs for panics or unhandled errors. Inspect `http_requests_total` in Grafana. |
| `HighLatency` | HTTP p99 latency > 2s over 5m | Warning | Profile slow handlers. Check Postgres and Elasticsearch response times. |

### Kafka Consumer Alerts

| Alert | Condition | Severity | Action |
|---|---|---|---|
| `KafkaConsumerLagHigh` | `kafka_consumer_lag > 1000` for 5m | Warning | Scale consumers or investigate slow processing. Check `kafka_message_processing_duration_seconds`. |
| `DLQMessagesIncreasing` | DLQ rate > 0 for 2m | Critical | Inspect failed messages in the DLQ topic. Fix deserialization or business logic errors. |
| `KafkaProcessingErrorsHigh` | Processing error rate > 10% for 3m | Warning | Check `kafka_processing_errors_total` labels for `error_type`. Investigate downstream services. |

### Elasticsearch Alerts

| Alert | Condition | Severity | Action |
|---|---|---|---|
| `ESIndexingErrorsHigh` | ES error rate > 10% for 3m | Critical | Check ES cluster health (`curl localhost:9200/_cluster/health`). Look for mapping conflicts or disk space issues. |
| `ESHighLatency` | ES p99 latency > 1s for 5m | Warning | Check ES JVM heap, shard count, and cluster load. Consider index optimisation. |

### PostgreSQL Alerts

| Alert | Condition | Severity | Action |
|---|---|---|---|
| `PostgresConnectionPoolExhausted` | Available connections < 2 for 1m | Critical | Check for connection leaks. Increase `POSTGRES_MAX_CONNS` in `.env`. Restart the app if connections are stuck. |

### Pipeline Alerts

| Alert | Condition | Severity | Action |
|---|---|---|---|
| `PipelineSyncLagHigh` | `pipeline_lag_seconds > 30` for 5m | Warning | CDC events are falling behind. Check Debezium connector status (`make check-debezium`). |
| `PipelineLastEventStale` | No CDC event for > 10m | Warning | Check Debezium connector and Kafka consumer group offsets. Verify WAL replication is active. |

---

## 5. Silencing an Alert During Planned Maintenance

Use the Alertmanager web UI or API to create a silence so notifications are suppressed during planned downtime.

### Via the Alertmanager UI

1. Open [http://localhost:9093](http://localhost:9093).
2. Click **Silences → New Silence**.
3. Set the **Start** and **End** times for your maintenance window.
4. Add matchers to target the specific alert(s), for example:
   - `alertname = ServiceDown`
   - `job = cdc-pipeline`
5. Add a comment explaining the reason (e.g. `Planned maintenance — upgrading Postgres`).
6. Click **Create**.

### Via the Alertmanager API

```bash
# Silence ServiceDown for 2 hours starting now
curl -X POST http://localhost:9093/api/v2/silences \
  -H 'Content-Type: application/json' \
  -d '{
    "matchers": [
      {"name": "alertname", "value": "ServiceDown", "isRegex": false}
    ],
    "startsAt": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'",
    "endsAt": "'$(date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+2H +%Y-%m-%dT%H:%M:%SZ)'",
    "createdBy": "operator",
    "comment": "Planned maintenance window"
  }'
```

### Listing and Deleting Silences

```bash
# List all active silences
curl -s http://localhost:9093/api/v2/silences | python3 -m json.tool

# Delete a silence by ID
curl -X DELETE http://localhost:9093/api/v2/silence/<SILENCE_ID>
```

> **Note:** Silences do not affect alert evaluation in Prometheus — alerts continue to be visible in `http://localhost:9090/alerts`. Only Alertmanager notifications are suppressed.
