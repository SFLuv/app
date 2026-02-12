import { ponder } from "ponder:registry";
import {
  transferAccount,
  allowance,
  approvalEvent,
  transferEvent,
} from "ponder:schema";
import { createTables, getHooks, PonderHook } from "./db";

createTables()

const parseAdminAddresses = () => {
  const raw = process.env.PAID_ADMIN_ADDRESSES || "";
  const list = raw
    .split(",")
    .map((addr) => addr.trim().toLowerCase())
    .filter((addr) => addr.length > 0);
  return Array.from(new Set(list));
};

const adminAddresses = parseAdminAddresses();
const w9TransactionUrl = process.env.W9_TRANSACTION_URL;

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

  try {
    const fromAddress = event.args.from.toLowerCase();
    if (w9TransactionUrl && adminAddresses.includes(fromAddress)) {
      await fetch(w9TransactionUrl, {
        method: "POST",
        body: JSON.stringify({
          from_address: event.args.from,
          to_address: event.args.to,
          hash: event.transaction.hash,
          amount: event.args.amount.toString(),
          timestamp: Number(event.block.timestamp),
        }),
        headers: {
          "Content-Type": "application/json",
          "X-Admin-Key": process.env.ADMIN_KEY as string,
        },
      });
    }
  } catch (error) {
    console.log("Error sending W9 transaction hook:", error);
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
