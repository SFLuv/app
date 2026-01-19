import { db } from "ponder:api";
import schema from "ponder:schema";
import { Context, Hono } from "hono";
import { client, graphql } from "ponder";
import { addHook, deleteHook, PonderHook } from "../db";

const app = new Hono();

app.use("/", async (c, next) => {
  const adminKey = process.env.ADMIN_KEY
  const authKey = c.req.header("X-Admin-Key")

  if(adminKey != authKey) {
    return c.status(401)
  }

  return await next()
})

app.use("/sql/*", client({ db, schema }));

app.use("/", graphql({ db, schema }));
app.use("/graphql", graphql({ db, schema }));

app.post("/hooks", async (c) => {
  const hookRequest = (await c.req.json()) as PonderHook

  try {
    const ping = await fetch(hookRequest.url, {
      headers: {
        "X-Admin-Key": process.env.ADMIN_KEY as string
      }
    })
    if(!ping.ok) {
      c.status(400)
      return c.text("bad request")
    }
    const hookResponse = await addHook(hookRequest)
    return c.json(hookResponse, 201)
  }
  catch(error) {
    console.log(error)
    c.status(500)
    return c.text("internal server error")
  }
})

app.delete("/hooks", async (c) => {
  const adminKey = process.env.ADMIN_KEY
  const authKey = c.req.header("X-Admin-Key")
  if(adminKey != authKey) {
    c.status(401)
    return c.text("bad auth key")
  }

  const hookId = Number(c.req.query("id"))

  try {
    await deleteHook(hookId)
    c.status(200)
    return c.text("ok")
  }
  catch(error) {
    console.log(error)
    c.status(500)
    return c.text("internal server error")
  }
})

export default app;
