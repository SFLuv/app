"use client"

import { Button } from "@/components/ui/button"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { cn } from "@/lib/utils"

interface PaginationProps {
  currentPage: number
  totalPages: number
  onPageChange: (page: number) => void
}

export function Pagination({ currentPage, totalPages, onPageChange }: PaginationProps) {
  const pages = Array.from({ length: totalPages }, (_, i) => i + 1)

  // Show a limited number of pages with ellipsis
  const getVisiblePages = () => {
    if (totalPages <= 7) {
      return pages
    }

    if (currentPage <= 3) {
      return [...pages.slice(0, 5), "ellipsis", totalPages]
    }

    if (currentPage >= totalPages - 2) {
      return [1, "ellipsis", ...pages.slice(totalPages - 5)]
    }

    return [1, "ellipsis", currentPage - 1, currentPage, currentPage + 1, "ellipsis", totalPages]
  }

  const visiblePages = getVisiblePages()

  return (
    <div className="flex items-center justify-center space-x-2">
      <Button
        variant="outline"
        size="icon"
        onClick={() => onPageChange(currentPage - 1)}
        disabled={currentPage === 1}
        className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
      >
        <ChevronLeft className="h-4 w-4" />
      </Button>

      {visiblePages.map((page, index) =>
        page === "ellipsis" ? (
          <span key={`ellipsis-${index}`} className="px-3 py-2 text-gray-500">
            ...
          </span>
        ) : (
          <Button
            key={`page-${page}`}
            variant={currentPage === page ? "default" : "outline"}
            size="icon"
            onClick={() => onPageChange(page as number)}
            className={cn(
              currentPage === page
                ? "bg-[#eb6c6c] hover:bg-[#d55c5c]"
                : "text-black dark:text-white bg-secondary hover:bg-secondary/80",
            )}
          >
            {page}
          </Button>
        ),
      )}

      <Button
        variant="outline"
        size="icon"
        onClick={() => onPageChange(currentPage + 1)}
        disabled={currentPage === totalPages}
        className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
      >
        <ChevronRight className="h-4 w-4" />
      </Button>
    </div>
  )
}
