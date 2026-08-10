import { Star } from "lucide-react"

import { cn } from "@/lib/utils"

interface RatingStarsProps {
  /** Out of five. Values outside 0–5 are clamped rather than overflowing. */
  rating: number
  /** Edge length of one star in pixels. */
  size?: number
  className?: string
}

const STAR_COUNT = 5

/**
 * A five-star rating, filled proportionally.
 *
 * Each star is drawn twice — an empty one, and a filled one clipped to the
 * fraction of that star the rating earns. Rounding to whole stars instead is
 * the obvious shortcut and it misreports: floor turns 4.9 into four stars, and
 * rounding turns 4.4 into four while 4.5 becomes five, so two merchants a
 * tenth of a point apart can look a whole star apart.
 *
 * The number is what screen readers get; the stars themselves are decorative.
 */
export function RatingStars({ rating, size = 16, className }: RatingStarsProps) {
  const clamped = Math.min(STAR_COUNT, Math.max(0, Number.isFinite(rating) ? rating : 0))

  return (
    <span
      className={cn("inline-flex items-center gap-0.5", className)}
      role="img"
      aria-label={`${clamped.toFixed(1)} out of ${STAR_COUNT} stars`}
    >
      {Array.from({ length: STAR_COUNT }, (_, index) => {
        // How much of THIS star is earned: 1 for a full one, 0 for an empty
        // one, and the remainder for the single star the rating lands inside.
        const fill = Math.min(1, Math.max(0, clamped - index))

        return (
          <span
            key={index}
            className="relative inline-block shrink-0"
            style={{ width: size, height: size }}
            aria-hidden
          >
            <Star className="absolute left-0 top-0 text-muted-foreground/30" width={size} height={size} />
            {fill > 0 ? (
              // Clipped by width, so the fill is a vertical wipe across the
              // star rather than a different icon.
              <span
                className="absolute left-0 top-0 overflow-hidden"
                style={{ width: size * fill, height: size }}
              >
                <Star className="text-yellow-400 fill-yellow-400" width={size} height={size} />
              </span>
            ) : null}
          </span>
        )
      })}
    </span>
  )
}
