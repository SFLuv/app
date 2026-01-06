# SFLUV Ponder Indexer

## Overview

SFLUV transaction indexer for historical lookup & webhook functionality.
All requests must contain an X-Admin-Key header that matches the admin key specified in .env.local


## Historical Lookup (DB Access)

The SFLUV Ponder Indexer saves historical transaction data to the specified db in the .env.local connection string.

Use ponder.schema.ts as the canonical db schema.

For app integration, create a db connection using the server url + "/sql" to submit queries, or fetch data using the graphql query endpoint at /graphql. Using graphql is recommended for most efficient queries due to custom db relations defined by ponder's orm.

## Webhook Integration

For webhook notifications, use the following api schema:

### POST "/hooks"

Creates an event listener that posts to "url" when a transaction to or from "address" is found.
NOTE: "id" in response body should be stored by client to be used for hook deletion. Hooks that have the same "address" and "url" will be automatically de-duped (only one notification sent to specified url). Keep an empty route open that returns OK upon a GET request to the callback url for hook registration validation.

Request Body:
```json
  {
    "address": "0x",
    "url": "http://callback.url"
  }
```

Response Body:
```json
  {
    "id": 1,
    "address": "0x",
    "url": "http://callback.url"
  }
```


Hook Body:
```json
  {
    "to": "0x",
    "from": "0x",
    "hash": "0x",
    "url": "http://callback.url",
    "amount": "10000"
  }
```
### DELETE /hooks?id={HOOK_ID}

Deletes event listener with "id".
