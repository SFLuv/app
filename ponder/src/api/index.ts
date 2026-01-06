import { db } from "ponder:api";
import schema from "ponder:schema";
import { Context, Hono } from "hono";
import { client, graphql } from "ponder";
import { addHook, deleteHook, PonderHook } from "../db";

const app = new Hono();

app.use("/sql/*", client({ db, schema }));

app.use("/", graphql({ db, schema }));
app.use("/graphql", graphql({ db, schema }));

app.post("/hooks", async (c) => {
  const adminKey = process.env.ADMIN_KEY
  const authKey = c.req.header("X-Admin-Key")
  if(adminKey != authKey) {
    return c.status(401)
  }

  const hookRequest = (await c.req.json()) as PonderHook

  try {
    const hookResponse = await addHook(hookRequest)
    const ping = await fetch(hookResponse.url, {
      method: "POST",
      body: JSON.stringify(hookResponse)
    })
    if(!ping.ok) {
      await deleteHook(hookResponse.id)
      return c.status(400)
    }
    return c.json(hookResponse, 201)
  }
  catch(error) {
    console.log(error)
    return c.status(500)
  }
})

app.delete("/hooks", async (c) => {
  const adminKey = process.env.ADMIN_KEY
  const authKey = c.req.header("X-Admin-Key")
  if(adminKey != authKey) {
    return c.status(401)
  }

  const hookId = Number(c.req.query("id"))

  try {
    await deleteHook(hookId)
    return c.status(200)
  }
  catch(error) {
    console.log(error)
    return c.status(500)
  }
})

export default app;
