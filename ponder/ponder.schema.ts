import { index, onchainTable, primaryKey, relations } from "ponder";

export const transferAccount = onchainTable(
  "transfer_account",
  (t) => ({
    chainId: t.integer().notNull(),
    address: t.hex().notNull(),
    balance: t.bigint().notNull(),
    isOwner: t.boolean().notNull(),
  }),
  (table) => ({
    pk: primaryKey({ columns: [table.chainId, table.address] }),
  }),
);

export const transferAccountRelations = relations(transferAccount, ({ many }) => ({
  transferFromEvents: many(transferEvent, { relationName: "from_account" }),
  transferToEvents: many(transferEvent, { relationName: "to_account" }),
}));

export const transferEvent = onchainTable(
  "transfer_event",
  (t) => ({
    id: t.text().notNull(),
    chainId: t.integer().notNull(),
    hash: t.hex().notNull(),
    amount: t.bigint().notNull(),
    timestamp: t.integer().notNull(),
    from: t.hex().notNull(),
    to: t.hex().notNull(),
  }),
  (table) => ({
    pk: primaryKey({ columns: [table.chainId, table.id] }),
    hashIdx: index("chain_hash_index").on(table.chainId, table.hash),
    fromIdx: index("chain_from_index").on(table.chainId, table.from),
    toIdx: index("chain_to_index").on(table.chainId, table.to),
  }),
);

export const transferEventRelations = relations(transferEvent, ({ one }) => ({
  fromAccount: one(transferAccount, {
    relationName: "from_account",
    fields: [transferEvent.chainId, transferEvent.from],
    references: [transferAccount.chainId, transferAccount.address],
  }),
  toAccount: one(transferAccount, {
    relationName: "to_account",
    fields: [transferEvent.chainId, transferEvent.to],
    references: [transferAccount.chainId, transferAccount.address],
  }),
}));

export const allowance = onchainTable(
  "allowance",
  (t) => ({
    chainId: t.integer().notNull(),
    owner: t.hex().notNull(),
    spender: t.hex().notNull(),
    amount: t.bigint().notNull(),
  }),
  (table) => ({
    pk: primaryKey({ columns: [table.chainId, table.owner, table.spender] }),
  }),
);

export const approvalEvent = onchainTable(
  "approval_event",
  (t) => ({
    id: t.text().notNull(),
    chainId: t.integer().notNull(),
    amount: t.bigint().notNull(),
    timestamp: t.integer().notNull(),
    owner: t.hex().notNull(),
    spender: t.hex().notNull(),
  }),
  (table) => ({
    pk: primaryKey({ columns: [table.chainId, table.id] }),
  }),
);
