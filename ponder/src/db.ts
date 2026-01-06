import pg from "pg"

const { Pool } = pg

const pool = new Pool({
  connectionString: process.env.DATABASE_URL
})

export interface PonderHook {
  id: number;
  address: string;
  url: string;
}

export const createTables = async () => {
  await pool.query(`
    CREATE TABLE IF NOT EXISTS ponder_hooks(
      id SERIAL PRIMARY KEY NOT NULL,
      address TEXT NOT NULL,
      url TEXT NOT NULL
    );

    CREATE INDEX IF NOT EXISTS hook_address ON ponder_hooks(address);
  `)
}

export const getHooks = async (address: string): Promise<PonderHook[]> => {
  return (await pool.query(`
    SELECT * FROM
      ponder_hooks
    WHERE
      address = $1;
    `, [address])).rows
}

export const addHook = async (hook: PonderHook): Promise<PonderHook> => {
  return (await pool.query(`
    INSERT INTO ponder_hooks(
      address,
      url
    ) VALUES (
      $1,
      $2
    )
    RETURNING *;
  `, [
    hook.address,
    hook.url
  ])).rows[0]
}

export const deleteHook = async (id: number) => {
  await pool.query(`
    DELETE FROM
      ponder_hooks
    WHERE
      id = $1;
  `, [id])
}