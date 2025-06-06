# Integrating with the SFLuv Faucet

** API Keys are issued only through direct contact with the SFLuv Admin Team at the moment. Please reach out to admin@sfluv.org if you'd like to discuss using the SFLuv faucet **

## Setup

To make any authenticated call to the SFLuv faucet APIs, attach your api key to your request with the X-API-KEY header.
Admin accounts are limited to a pre-determined weekly budget. By default, budgets reset every Monday morning at 12am PST.

The base URL for our faucet is https://app.sfluv.org/api/faucet

## Relevant Endpoints

### Create an Event
#### POST /events

Make a POST request to the /events endpoint to create a new reward event.
Returns event_id

Request Fields:

```javascript
  {
    "title": "string", // Event title for record storage
    "description": "string", // Event description for record storage
    "codes": int, // Number of codes for event
    "amount": int, // Value of each code for event
    // NOTE: the value of codes * amount will be subtracted from your week's budget upon a successful event generation
    "expiration": int // Expiration date for event codes (cannot exceed next admin refresh period)
  }
```

Response:

```txt
  this-is-an-example-uuid
```
Example uuid will be returned in plaintext.

#### GET /events

Make a GET request to the /events endpoint to get code for a given event.
Returns event_id

Request Query Params:

  event: The event ID for which you want to get codes.
  page: The page number if pagination is required (optional, default 0)
  count: The number of items per page if pagination is required (optional, default 100)



Example event request:
```bash
  curl https://app.sfluv.org/api/faucet/events?event=this-is-an-example-uuid&page=0&count=100
```

Response:

```javascript
  [
    {
      "id":"this-is-another-example-uuid", // The id of the code to be used for redemption
      "redeemed":false,
      "event":"this-is-an-example-uuid" // The event that this code belongs to
    },
    {
      "id":"this-is-athird-example-uuid",
      "redeemed":true,
      "event":"this-is-an-example-uuid"
    }
  ]
```
Example uuid will be returned in plaintext.


To create a redeem link, format the returned code uuids as follows:

```txt
  https://app.citizenwallet.xyz/#/?dl=plugin&alias=wallet.sfluv.org&plugin=https%3A%2F%2Fapp.sfluv.org%3Fcode%3D{uuid}%26page%3Dredeem
```