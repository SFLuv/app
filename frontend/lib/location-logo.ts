/**
 * The square-crop machinery behind a location's logo.
 *
 * Shared, not duplicated, because there are two places a merchant sets one —
 * the Location Approval Form's first step, and the icon card in Settings — and
 * they must agree on the output exactly. The crop decides what appears inside
 * the map pin's circle; two implementations drifting apart would mean a logo
 * that framed correctly at application time and wrongly when it was replaced.
 *
 * Everything here is pure and DOM-only. Uploading is deliberately not part of
 * it: the stepper has no location id to upload to until the listing exists, so
 * it holds the blob and posts it afterwards, while the settings card posts
 * immediately. Those are different flows around one identical crop.
 */

/** Edge length of the square the crop is saved at. */
export const LOGO_OUTPUT_SIZE = 512

/** Edge length of the on-screen crop window. */
export const LOGO_VIEWPORT = 260

/**
 * The server's own ceiling, mirrored. The crop is re-encoded before upload so
 * this bounds the PNG that leaves the browser, not the file picked.
 */
export const LOGO_MAX_UPLOAD_BYTES = 2 * 1024 * 1024

/**
 * What the file picker will take. Wider than what the server stores — GIF is
 * accepted here because the browser can decode one and the crop re-encodes it
 * to PNG, so the merchant never learns their file was the wrong kind for a
 * reason that stopped being true the moment it was cropped.
 */
export const LOGO_ACCEPTED_FILE_TYPES = /^image\/(png|jpeg|webp|gif)$/

/** The `accept` attribute for the picker, which mirrors what the server takes. */
export const LOGO_FILE_INPUT_ACCEPT = "image/png,image/jpeg,image/webp"

export interface LogoCrop {
  /** Multiplier on top of the scale that just covers the viewport. */
  zoom: number
  /** Top-left of the scaled image relative to the viewport, in CSS pixels. */
  x: number
  y: number
}

/** Scale at which the image exactly covers the crop window. */
export const logoBaseScale = (image: HTMLImageElement | null): number => {
  if (!image) return 1
  return LOGO_VIEWPORT / Math.min(image.naturalWidth, image.naturalHeight)
}

/**
 * Holds the image over the whole crop window.
 *
 * The image must always cover it — never let a gap open at an edge, or the
 * saved crop would contain transparent bands down one side.
 */
export const clampLogoCrop = (image: HTMLImageElement | null, next: LogoCrop): LogoCrop => {
  if (!image) return next
  const scale = logoBaseScale(image) * next.zoom
  const width = image.naturalWidth * scale
  const height = image.naturalHeight * scale
  return {
    zoom: next.zoom,
    x: Math.min(0, Math.max(LOGO_VIEWPORT - width, next.x)),
    y: Math.min(0, Math.max(LOGO_VIEWPORT - height, next.y)),
  }
}

/** Centred at the cover scale, which is the crop most people want. */
export const initialLogoCrop = (image: HTMLImageElement): LogoCrop => {
  const scale = logoBaseScale(image)
  return {
    zoom: 1,
    x: (LOGO_VIEWPORT - image.naturalWidth * scale) / 2,
    y: (LOGO_VIEWPORT - image.naturalHeight * scale) / 2,
  }
}

/** Zoom about the centre of the window, so the subject does not drift away. */
export const zoomLogoCrop = (
  image: HTMLImageElement | null,
  current: LogoCrop,
  zoom: number,
): LogoCrop => {
  const ratio = zoom / current.zoom
  const centre = LOGO_VIEWPORT / 2
  return clampLogoCrop(image, {
    zoom,
    x: centre - (centre - current.x) * ratio,
    y: centre - (centre - current.y) * ratio,
  })
}

/**
 * The on-screen size of the scaled image, for positioning the drag layer.
 *
 * Whatever renders these numbers must also carry `max-w-none`. Tailwind's
 * preflight sets `img { max-width: 100% }`, and an inline `width` does not
 * override a `max-width` — so the width gets clamped to the crop window while
 * the inline height goes on growing, and the preview stretches vertically as it
 * is zoomed. `cropLogoToPngBlob` below works from these same numbers as
 * arithmetic rather than from the DOM, so the saved image stays correct and
 * only the preview lies, which is the hard version of this bug to spot: the
 * merchant frames one thing and the map shows another.
 */
export const logoRenderedSize = (image: HTMLImageElement | null, crop: LogoCrop) => {
  const scale = logoBaseScale(image) * crop.zoom
  return {
    width: image ? image.naturalWidth * scale : 0,
    height: image ? image.naturalHeight * scale : 0,
  }
}

/**
 * Decodes a picked file, or rejects with a message meant for the merchant.
 *
 * The size ceiling here is deliberately generous and is not the upload limit:
 * the crop is re-encoded before it is sent, so a large original is fine. This
 * only turns away files big enough to stall the browser decoding them.
 */
export const readLogoFile = async (file: File): Promise<HTMLImageElement> => {
  if (!LOGO_ACCEPTED_FILE_TYPES.test(file.type)) {
    throw new Error("Choose a PNG, JPEG, or WebP image.")
  }
  if (file.size > LOGO_MAX_UPLOAD_BYTES * 4) {
    throw new Error("That image is too large. Try one under 8 MB.")
  }

  const url = URL.createObjectURL(file)
  try {
    return await new Promise<HTMLImageElement>((resolve, reject) => {
      const element = new window.Image()
      element.onload = () => resolve(element)
      element.onerror = () => reject(new Error("That image could not be read."))
      element.src = url
    })
  } catch (error) {
    // Only on failure. On success the caller owns the URL — it is the `src` of
    // the drag layer — and revokes it when it replaces or unmounts.
    URL.revokeObjectURL(url)
    throw error
  }
}

/**
 * Renders the framed square to a PNG blob.
 *
 * Maps the window back onto the source image: the visible square starts at the
 * negated offset and is one window wide at the current scale.
 */
export const cropLogoToPngBlob = async (
  image: HTMLImageElement | null,
  crop: LogoCrop,
): Promise<Blob | null> => {
  if (!image) return null

  const canvas = document.createElement("canvas")
  canvas.width = LOGO_OUTPUT_SIZE
  canvas.height = LOGO_OUTPUT_SIZE
  const context = canvas.getContext("2d")
  if (!context) return null

  const scale = logoBaseScale(image) * crop.zoom
  const sourceSize = LOGO_VIEWPORT / scale
  context.imageSmoothingQuality = "high"
  context.drawImage(
    image,
    -crop.x / scale,
    -crop.y / scale,
    sourceSize,
    sourceSize,
    0,
    0,
    LOGO_OUTPUT_SIZE,
    LOGO_OUTPUT_SIZE,
  )

  return await new Promise((resolve) => canvas.toBlob(resolve, "image/png"))
}

/**
 * Uploads a cropped logo to the location it belongs to.
 *
 * The endpoint is addressed by location id and by nothing else: a logo belongs
 * to one listing, not to the merchant who owns it, so a merchant with three
 * shops sets three logos and replacing one leaves the other two alone.
 *
 * Returns the versioned URL the bytes are published under. The version stamp
 * matters — icons are served with a long cache lifetime, so a replacement needs
 * a different URL or viewers keep seeing the old one.
 */
export const uploadLocationLogo = async (
  authFetch: (endpoint: string, options?: RequestInit) => Promise<Response>,
  locationId: number,
  blob: Blob,
): Promise<string> => {
  if (blob.size > LOGO_MAX_UPLOAD_BYTES) {
    throw new Error("That crop is too detailed to upload. Try a simpler image.")
  }

  const body = new FormData()
  body.append("icon", blob, "icon.png")

  const response = await authFetch(`/locations/${locationId}/icon`, { method: "POST", body })
  if (!response.ok) {
    const payload = await response.json().catch(() => null)
    throw new Error(payload?.error || "Unable to save your logo.")
  }

  const payload = (await response.json()) as { icon_url?: string }
  return payload.icon_url ?? ""
}
