// URL-driven tabs and segmented controls: each option is a router link, so
// the choice lives in the URL (reload, share, back) and the current one is
// marked aria-current="page" by TanStack.
import type { ReactNode } from 'react'
import { cn } from './cn'
import { HrefLink } from './link'

export type TabItem = { href: string; label: ReactNode; exact?: boolean }

const VARIANT = {
  underline: {
    list: 'flex gap-1 overflow-x-auto border-b border-line',
    item: 'inline-flex min-h-10 shrink-0 items-center gap-1.5 border-b-2 border-transparent px-3 text-sm font-medium whitespace-nowrap text-stone-500 -mb-px hover:text-stone-900 data-[status=active]:border-accent data-[status=active]:text-stone-900',
  },
  segmented: {
    list: 'inline-flex shrink-0 rounded-lg bg-stone-100 p-0.5 ring-1 ring-stone-200 ring-inset',
    item: 'inline-flex min-h-9 items-center rounded-md px-3.5 text-xs font-medium whitespace-nowrap text-stone-500 hover:text-stone-900 md:min-h-8 data-[status=active]:bg-white data-[status=active]:font-semibold data-[status=active]:text-accent-strong data-[status=active]:shadow-sm data-[status=active]:ring-1 data-[status=active]:ring-stone-200',
  },
} as const

/**
 * Router link tabs. `exact: false` keeps a tab current on sub-paths and any
 * query (activeOptions `{ exact: false, includeSearch: false }`); by default
 * the tab is current only when path and query match its href exactly.
 */
export function Tabs({ label, items, variant = 'underline', className }: { label: string; items: TabItem[]; variant?: keyof typeof VARIANT; className?: string }) {
  const style = VARIANT[variant]
  return (
    <nav aria-label={label} className={cn(variant === 'segmented' ? 'inline-flex' : undefined, className)}>
      <ul className={style.list}>
        {items.map((item) => (
          <li key={item.href} className="flex">
            <HrefLink
              href={item.href}
              activeOptions={item.exact === false ? { exact: false, includeSearch: false } : undefined}
              className={style.item}
            >
              {item.label}
            </HrefLink>
          </li>
        ))}
      </ul>
    </nav>
  )
}
