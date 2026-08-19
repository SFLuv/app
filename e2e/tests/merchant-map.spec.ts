/**
 * The merchant map, and the multi-location case in particular.
 *
 * A merchant with two shops is the least proven code in the repo: each location
 * derives its own payment wallet, and migration 1.40 added unique indexes to
 * stop two shops pointing at the same till. The API scenario asserts the wallets
 * differ; what only a browser can tell us is whether both shops actually reach
 * the map a customer looks at.
 *
 * An approved location that never renders is invisible to every customer while
 * looking perfectly healthy in the database.
 */
import { psql } from "../lib/harness"
import { expect, test } from "../lib/test"

/** Approved locations, newest first — what the map is supposed to be showing. */
function approvedLocationNames(limit: number): string[] {
  const rows = psql(
    `SELECT name FROM locations
      WHERE approval = TRUE AND name IS NOT NULL AND TRIM(name) <> ''
      ORDER BY id DESC LIMIT ${limit};`,
  )
  return rows ? rows.split("\n").map((r) => r.trim()).filter(Boolean) : []
}

test("the map page renders", async ({ page }) => {
  await page.goto("/map")

  await expect(page.getByRole("heading", { name: "Merchant Map" })).toBeVisible()
  await expect(page.getByText("Places that accept SFLuv.")).toBeVisible()
  /**
   * Assert the filter's VALUE, not its placeholder. SelectValue only renders
   * the placeholder when nothing is selected, and this one defaults to
   * "All Locations" (app/map/page.tsx:20) — so "Filter by type" is never on
   * screen in the normal case.
   */
  await expect(page.getByRole("combobox")).toContainText("All Locations")
})

test("an approved location is reachable from the map's merchant list", async ({ page }) => {
  const names = approvedLocationNames(1)
  test.skip(names.length === 0, "no approved locations in this database")
  const name = names[0]

  await page.goto("/map")
  await expect(page.getByRole("heading", { name: "Merchant Map" })).toBeVisible()

  /**
   * The map carries its own searchable merchant list beside it — there is no
   * separate list view any more (app/map/page.tsx:107). Search rather than
   * scroll: the production clone has hundreds of locations and the one we want
   * is unlikely to be on screen.
   */
  const search = page.getByRole("textbox").first()
  await search.fill(name.slice(0, 24))

  await expect(
    page.getByText(name, { exact: false }).first(),
    `approved location "${name}" should be findable on the map`,
  ).toBeVisible({ timeout: 15_000 })
})

test("both shops of a multi-location merchant reach the map", async ({ page }) => {
  /**
   * Two approved locations owned by the same person. If only one renders, a
   * merchant's second shop is invisible to customers — the failure the
   * multi-location work exists to avoid, and one no API assertion can see.
   */
  const owner = psql(
    `SELECT owner_id FROM locations
      WHERE approval = TRUE AND owner_id IS NOT NULL
      GROUP BY owner_id HAVING COUNT(*) > 1
      ORDER BY MAX(id) DESC LIMIT 1;`,
  )
  test.skip(!owner, "no owner has two approved locations in this database")

  const names = psql(
    `SELECT name FROM locations
      WHERE approval = TRUE AND owner_id = '${owner}'
        AND name IS NOT NULL AND TRIM(name) <> ''
      ORDER BY id DESC LIMIT 2;`,
  )
    .split("\n")
    .map((r) => r.trim())
    .filter(Boolean)

  test.skip(names.length < 2, "owner does not have two named approved locations")

  await page.goto("/map")
  await expect(page.getByRole("heading", { name: "Merchant Map" })).toBeVisible()
  const search = page.getByRole("textbox").first()

  for (const name of names) {
    await search.fill("")
    await search.fill(name.slice(0, 24))
    await expect(
      page.getByText(name, { exact: false }).first(),
      `"${name}" should be on the map — both shops of one merchant must appear`,
    ).toBeVisible({ timeout: 15_000 })
  }

  console.log(`  both shops visible: ${names.join("  |  ")}`)
})
