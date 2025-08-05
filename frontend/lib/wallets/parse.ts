const EXAMPLE_STRING = "https://wallet.sfluv.org/#/wallet/v4-MHhEM2ZkNkM1OTU1YzhlNzIzY0NiMjRlQUMwMDg3RDJDNzg3QzBiNzlDfDB4NWU5ODdhNmM0YmI0MjM5ZDQ5OEU3OGMzNGU5ODZhY2YyOWM4MUU4ZXx7ImFkZHJlc3MiOiJmODdjOWU1N2FlNmEzM2FlMjkzMmEzMWQwNGUwNWYwZDhjMDZjZmI5IiwiaWQiOiJjMzNjNTgxZS0yZDZiLTRlZDUtYTAxNS03ZjM2ZGIzYTUxMzciLCJ2ZXJzaW9uIjozLCJDcnlwdG8iOnsiY2lwaGVyIjoiYWVzLTEyOC1jdHIiLCJjaXBoZXJwYXJhbXMiOnsiaXYiOiI1MmFmZTI1YTI1ZTlkYmJmOGVjMDI0ODUxM2IxMTA5NSJ9LCJjaXBoZXJ0ZXh0IjoiOGYwZmViMzk2MjFmMTU0NzU5MjY3NmQyY2YyNzU3YTZlMzAzYWZmYzI2NjA0OGQzOTUyN2IyNzBhM2MxNTQ4MiIsImtkZiI6InNjcnlwdCIsImtkZnBhcmFtcyI6eyJzYWx0IjoiYmU3MTExNGMzZTRiYTk0N2FlNDlhYzUwZDVlNGQ1NTlkMTZhYTcwZjJmMGFiYThlYzA0MjFkODM4ZmRmMDA0MCIsIm4iOjEzMTA3MiwiZGtsZW4iOjMyLCJwIjoxLCJyIjo4fSwibWFjIjoiMGZlMTVhMWMxMTI2Y2NjY2JlYjY0NjAzZjA2YmIyOTRmMjQ3M2JiNWVjMTkxZGVkOTE5Njc0YjliYjljZDVkNCJ9LCJ4LWV0aGVycyI6eyJjbGllbnQiOiJldGhlcnMvNi4xMy41IiwiZ2V0aEZpbGVuYW1lIjoiVVRDLS0yMDI1LTA4LTA0VDIyLTQ0LTA5LjBaLS1mODdjOWU1N2FlNmEzM2FlMjkzMmEzMWQwNGUwNWYwZDhjMDZjZmI5IiwicGF0aCI6Im0vNDQnLzYwJy8wJy8wLzAiLCJsb2NhbGUiOiJlbiIsIm1uZW1vbmljQ291bnRlciI6ImQ4MDMxZTEzNmE2YjQyMjJjNzM5ZDIzOGUyYjFkMGQ1IiwibW5lbW9uaWNDaXBoZXJ0ZXh0IjoiYTkzMzNhZDgwNzMwNmZlZDA0MmEwODcxZDQzMjdmYjciLCJ2ZXJzaW9uIjoiMC4xIn19?alias=wallet.sfluv.org"

const parseKeyFromCWLink = (walletLink: string): string | null => {
  try {
    let url = new URL(walletLink)
    console.log(url)
    let walletString = url.hash.split("/")[2].split("?")[0].slice(3)
    console.log(walletString)
    let decoded = atob(walletString)
    console.log(decoded)
    let obj = JSON.parse(decoded)
    console.log(obj)
    return decoded
  }
  catch(error) {
    console.log(error)
    return null
  }
}

parseKeyFromCWLink(EXAMPLE_STRING)