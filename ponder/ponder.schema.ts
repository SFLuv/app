import { index, onchainTable, primaryKey, relations } from "ponder";

export const transferAccount = onchainTable("transfer_account", (t) => ({
  address: t.hex().primaryKey(),
  balance: t.bigint().notNull(),
  isOwner: t.boolean().notNull(),
}));

export const transferAccountRelations = relations(transferAccount, ({ many }) => ({
  transferFromEvents: many(transferEvent, { relationName: "from_account" }),
  transferToEvents: many(transferEvent, { relationName: "to_account" }),
}));

export const transferEvent = onchainTable(
  "transfer_event",
  (t) => ({
    id: t.text().primaryKey(),
    hash: t.hex().notNull(),
    amount: t.bigint().notNull(),
    timestamp: t.integer().notNull(),
    from: t.hex().notNull(),
    to: t.hex().notNull(),
  }),
  (table) => ({
    hashIdx: index("hash_index").on(table.hash),
    fromIdx: index("from_index").on(table.from),
    toIdx: index("to_index").on(table.to),
  }),
);

export const transferEventRelations = relations(transferEvent, ({ one }) => ({
  fromAccount: one(transferAccount, {
    relationName: "from_account",
    fields: [transferEvent.from],
    references: [transferAccount.address],
  }),
  toAccount: one(transferAccount, {
    relationName: "to_account",
    fields: [transferEvent.to],
    references: [transferAccount.address],
  }),
}));

export const allowance = onchainTable(
  "allowance",
  (t) => ({
    owner: t.hex(),
    spender: t.hex(),
    amount: t.bigint().notNull(),
  }),
  (table) => ({
    pk: primaryKey({ columns: [table.owner, table.spender] }),
  }),
);

export const approvalEvent = onchainTable("approval_event", (t) => ({
  id: t.text().primaryKey(),
  amount: t.bigint().notNull(),
  timestamp: t.integer().notNull(),
  owner: t.hex().notNull(),
  spender: t.hex().notNull(),
}));
