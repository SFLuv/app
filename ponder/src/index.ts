import { ponder } from "ponder:registry";
import {
  transferAccount,
  allowance,
  approvalEvent,
  transferEvent,
} from "ponder:schema";
import { createTables, getHooks, PonderHook } from "./db";

createTables()

ponder.on("ERC20:Transfer", async ({ event, context }) => {
  await context.db
    .insert(transferAccount)
    .values({ address: event.args.from, balance: 0n, isOwner: false })
    .onConflictDoUpdate((row) => ({
      balance: row.balance - event.args.amount,
    }));

  await context.db
    .insert(transferAccount)
    .values({
      address: event.args.to,
      balance: event.args.amount,
      isOwner: false,
    })
    .onConflictDoUpdate((row) => ({
      balance: row.balance + event.args.amount,
    }));

  // add row to "transfer_event".
  await context.db.insert(transferEvent).values({
    id: event.id,
    hash: event.transaction.hash,
    amount: event.args.amount,
    timestamp: Number(event.block.timestamp),
    from: event.args.from,
    to: event.args.to,
  });

  try {
    let deduped: Record<string, boolean> = {};
    (await Promise.all([
      getHooks(event.args.from),
      event.args.to === event.args.from ? undefined : getHooks(event.args.to)
    ]))
    .map((set: PonderHook[] | undefined) => {
      if(!set) return
      set.forEach(async (hook) => {
        try {
          const hookBody = {
            to: event.args.to,
            from: event.args.from,
            hash: event.transaction.hash,
            amount: event.args.amount.toString()
          }

          if(deduped[hook.url]) return
          deduped[hook.url] = true
          await fetch(hook.url, {
            method: "POST",
            body: JSON.stringify(hookBody),
            headers: {
              "X-Admin-Key": process.env.ADMIN_KEY as string
            }
          })
        }
        catch {
          console.log("Error sending hook for tx " + event.transaction.hash + ":", hook)
        }
      })
    })
  }
  catch {
    console.log("Error getting hooks for transfer:", event)
  }
});

ponder.on("ERC20:Approval", async ({ event, context }) => {
  // upsert "allowance".
  await context.db
    .insert(allowance)
    .values({
      spender: event.args.spender,
      owner: event.args.owner,
      amount: event.args.amount,
    })
    .onConflictDoUpdate({ amount: event.args.amount });

  // add row to "approval_event".
  await context.db.insert(approvalEvent).values({
    id: event.id,
    amount: event.args.amount,
    timestamp: Number(event.block.timestamp),
    owner: event.args.owner,
    spender: event.args.spender,
  });
});
