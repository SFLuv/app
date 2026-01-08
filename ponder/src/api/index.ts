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

  return next()
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
      return c.status(400)
    }
    const hookResponse = await addHook(hookRequest)
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
