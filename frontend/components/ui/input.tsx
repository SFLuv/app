import * as React from "react"

import { cn } from "@/lib/utils"

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, onWheel, onChange, onBlur, onFocus, value, ...props }, ref) => {
    const innerRef = React.useRef<HTMLInputElement | null>(null)
    const isNumber = type === "number"
    const isControlled = value !== undefined

    // While a number field is focused, show exactly what the user typed.
    //
    // Without this, a controlled input whose parent does Number(e.target.value)
    // turns an emptied field into 0 and immediately re-renders "0" — so clearing
    // the box to type "40" leaves you fighting a leading zero ("040"). The draft
    // is dropped on blur, so the parent's canonical value takes over again and
    // nothing downstream has to change.
    const [draft, setDraft] = React.useState<string | null>(null)

    const setRefs = React.useCallback(
      (node: HTMLInputElement | null) => {
        innerRef.current = node
        if (typeof ref === "function") ref(node)
        else if (ref) (ref as React.MutableRefObject<HTMLInputElement | null>).current = node
      },
      [ref],
    )

    const handleWheel = (event: React.WheelEvent<HTMLInputElement>) => {
      // A focused number input consumes wheel events, so scrolling the page
      // silently edits the value. That is almost never intended and the change
      // is easy to miss, so blur instead and let the page scroll.
      if (isNumber && document.activeElement === innerRef.current) {
        innerRef.current?.blur()
      }
      onWheel?.(event)
    }

    const handleChange = (event: React.ChangeEvent<HTMLInputElement>) => {
      if (isNumber && isControlled) setDraft(event.target.value)
      onChange?.(event)
    }

    const handleBlur = (event: React.FocusEvent<HTMLInputElement>) => {
      setDraft(null)
      onBlur?.(event)
    }

    const handleFocus = (event: React.FocusEvent<HTMLInputElement>) => {
      if (isNumber && isControlled) setDraft(String(value ?? ""))
      onFocus?.(event)
    }

    return (
      <input
        type={type}
        className={cn(
          "flex h-10 w-full rounded-lg border border-input bg-card/90 px-3 py-2 text-sm shadow-sm ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:border-primary/55 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/35 focus-visible:ring-offset-0 disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
        ref={setRefs}
        value={isNumber && isControlled && draft !== null ? draft : value}
        onWheel={handleWheel}
        onChange={handleChange}
        onBlur={handleBlur}
        onFocus={handleFocus}
        {...props}
      />
    )
  },
)
Input.displayName = "Input"

export { Input }
