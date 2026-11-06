// Popups that hang off one control: an overflow menu (destructive and rare
// actions out of the way) and a tooltip (why a control is disabled).
import { MoreHorizontal } from 'lucide-react'
import { DropdownMenu, Tooltip as RTooltip } from 'radix-ui'
import type { ReactElement, ReactNode } from 'react'
import { buttonClasses } from './button'
import { cn } from './cn'

export type MenuItem = { label: string; icon?: ReactNode; onSelect: () => void; tone?: 'danger'; disabled?: boolean }

/**
 * A "…" button opening a Radix dropdown menu (arrow keys, typeahead, Escape).
 * `label` names the trigger for screen readers ("投稿の操作").
 */
export function Menu({ label, items, align = 'end', className }: { label: string; items: MenuItem[]; align?: 'start' | 'end'; className?: string }) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger aria-label={label} title={label} className={buttonClasses('ghost', 'sm', cn('relative z-[1] w-10 px-0 text-stone-500 md:w-8', className))}>
        <MoreHorizontal aria-hidden />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align={align}
          sideOffset={4}
          className="z-50 min-w-44 rounded-lg border border-line bg-white p-1 text-sm shadow-lg animate-pop-in focus:outline-none"
        >
          {items.map((item) => (
            <DropdownMenu.Item
              key={item.label}
              disabled={item.disabled}
              onSelect={item.onSelect}
              className={cn(
                'flex min-h-10 cursor-default items-center gap-2 rounded-md px-2.5 outline-none select-none md:min-h-8 data-[disabled]:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0',
                item.tone === 'danger' ? 'text-bad-ink data-[highlighted]:bg-bad-soft' : 'text-stone-700 data-[highlighted]:bg-stone-100'
              )}
            >
              {item.icon ? <span aria-hidden className="contents">{item.icon}</span> : null}
              {item.label}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

/**
 * Hover / focus hint for `children` (one element that takes a ref and focus).
 * A disabled button gets no pointer or focus events: wrap it in a focusable
 * `<span tabIndex={0}>` and pass that. The hint is supplementary; keep the
 * same text in `aria-describedby` or visible copy when it matters.
 */
export function Tooltip({ content, children, side = 'top' }: { content: ReactNode; children: ReactElement; side?: 'top' | 'bottom' | 'left' | 'right' }) {
  return (
    <RTooltip.Provider delayDuration={200}>
      <RTooltip.Root>
        <RTooltip.Trigger asChild>{children}</RTooltip.Trigger>
        <RTooltip.Portal>
          <RTooltip.Content
            side={side}
            sideOffset={6}
            className="z-50 max-w-64 rounded-md bg-stone-900 px-2.5 py-1.5 text-xs leading-snug text-white shadow-md animate-fade-in"
          >
            {content}
            <RTooltip.Arrow className="fill-stone-900" />
          </RTooltip.Content>
        </RTooltip.Portal>
      </RTooltip.Root>
    </RTooltip.Provider>
  )
}
