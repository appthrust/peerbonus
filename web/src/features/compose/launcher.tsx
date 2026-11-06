import { Gift, X } from 'lucide-react'
import { useEffect, useEffectEvent, useId, useRef, useState, useSyncExternalStore } from 'react'
import type { Post } from '../../api/peerbonus'
import { useText } from '../../i18n'
import { useSession } from '../../shell/use-session'
import { Avatar, Dialog, IconButton, Notice } from '../../ui'
import { ComposeForm, type ComposeDraft } from './compose-form'

/** How long the "sent" toast stays. */
const TOAST_MS = 4_000
/** Tailwind's `md`: from here the form opens in place, below it as a bottom sheet. */
const WIDE = '(min-width: 48rem)'

function subscribeWide(onChange: () => void) {
  const query = window.matchMedia(WIDE)
  query.addEventListener('change', onChange)
  return () => query.removeEventListener('change', onChange)
}

/**
 * The timeline's one way to send. Closed: a row that reads like an input (the viewer's avatar,
 * 「だれかに感謝を贈る…」; clicking or tabbing to it opens the form) on wider screens, a floating button bottom-right on phones. Open: the
 * form in place on wider screens, a bottom sheet on phones. `to` (`?to=`) opens it with that
 * recipient; `open` / `onOpenChange` let the page open it from elsewhere (empty states, the
 * reminder). A send closes it, shows "sent" for four seconds and calls `onSent(post)` so the page
 * can put the post on top of the feed. The draft survives closing without sending.
 */
export function ComposeLauncher({
  onSent,
  to,
  onPreselected,
  defaultOpen = false,
  open: openProp,
  onOpenChange,
}: {
  onSent?: (post: Post) => void
  to?: string
  onPreselected?: () => void
  defaultOpen?: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const { t } = useText()
  const { me } = useSession()
  const headingId = useId()
  // Static markup (tests, first paint) lays out the wide form; phones switch right after.
  const wide = useSyncExternalStore(subscribeWide, () => window.matchMedia(WIDE).matches, () => true)
  const [openState, setOpenState] = useState(defaultOpen || Boolean(to))
  const open = openProp ?? openState
  const [toast, setToast] = useState<{ name: string; points: number } | null>(null)
  const [draft, setDraft] = useState<ComposeDraft | null>(null)
  const row = useRef<HTMLButtonElement>(null)
  // Why the form just closed: focus goes back to the row, and after a send the new post (right below) comes into view.
  const closedBy = useRef<'cancel' | 'sent' | null>(null)
  // Focus on the row opens the form, except focus that only comes back to it: after closing, or the window regaining focus.
  const focusReturns = useRef(false)

  const setOpen = (next: boolean) => {
    setOpenState(next)
    onOpenChange?.(next)
  }

  const openForRecipient = useEffectEvent(() => {
    setOpenState(true)
    onOpenChange?.(true)
  })
  // 「贈る」 elsewhere (`?to=`) opens the form even when it was closed.
  useEffect(() => {
    if (to) openForRecipient()
  }, [to])

  useEffect(() => {
    const onWindowBlur = () => {
      if (document.activeElement === row.current) focusReturns.current = true
    }
    window.addEventListener('blur', onWindowBlur)
    return () => window.removeEventListener('blur', onWindowBlur)
  }, [])

  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(null), TOAST_MS)
    return () => clearTimeout(timer)
  }, [toast])

  useEffect(() => {
    const reason = closedBy.current
    if (open || !reason) return
    closedBy.current = null
    if (!wide) {
      // The sheet returns focus itself; the feed's top (the new post) is what matters now.
      if (reason === 'sent') window.scrollTo({ top: 0, behavior: 'smooth' })
      return
    }
    focusReturns.current = true
    row.current?.focus({ preventScroll: true })
    if (reason === 'sent') row.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }, [open, wide])

  const close = (reason: 'cancel' | 'sent' = 'cancel') => {
    closedBy.current = reason
    setOpen(false)
  }
  const sent = (post: Post) => {
    setToast({ name: post.recipient.displayName, points: post.points })
    close('sent')
    onSent?.(post)
  }
  const form = (
    <ComposeForm
      to={to}
      onPreselected={onPreselected}
      onSent={sent}
      onCancel={() => close()}
      autoFocus
      draft={draft}
      onDraftChange={setDraft}
      labelledBy={headingId}
    />
  )

  return (
    <>
      {open && wide ? (
        <section
          aria-labelledby={headingId}
          onKeyDown={(event) => {
            if (event.key === 'Escape' && !event.defaultPrevented) close()
          }}
          className="rounded-lg border border-accent-line bg-white px-4 pt-3 pb-5 shadow-sm animate-pop-in sm:px-5"
        >
          <div className="mb-3 flex items-center justify-between gap-3">
            <h2 id={headingId} className="flex items-center gap-2 text-base font-semibold text-stone-900">
              <Gift aria-hidden className="size-4 text-accent" />
              {t('peerbonus.composer.title')}
            </h2>
            <IconButton tone="ghost" size="sm" icon={<X />} label={t('peerbonus.composer.collapse')} onClick={() => close()} />
          </div>
          {form}
        </section>
      ) : (
        <button
          ref={row}
          type="button"
          onClick={() => setOpen(true)}
          onFocus={() => {
            if (focusReturns.current) focusReturns.current = false
            else setOpen(true)
          }}
          className="group flex w-full scroll-mt-20 items-center gap-3 rounded-lg border border-line bg-white px-3 py-2.5 text-left shadow-xs transition-colors hover:border-accent-line hover:bg-accent-soft/40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent max-md:hidden"
        >
          <Avatar name={me.member.displayName} avatarUrl={me.member.avatarUrl} size="md" />
          <span className="min-w-0 flex-1 truncate text-sm text-stone-500 group-hover:text-stone-700">{t('peerbonus.composer.launcher')}</span>
          <span aria-hidden className="grid size-8 shrink-0 place-items-center rounded-full bg-accent text-white">
            <Gift className="size-4" />
          </span>
        </button>
      )}
      {!wide ? (
        <>
          <button
            type="button"
            aria-label={t('peerbonus.composer.title')}
            onClick={() => setOpen(true)}
            className="fixed right-4 bottom-[calc(1rem+env(safe-area-inset-bottom))] z-30 grid size-14 place-items-center rounded-full bg-accent text-white shadow-lg ring-4 ring-white/70 transition-transform active:scale-95 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent md:hidden"
          >
            <Gift aria-hidden className="size-6" />
          </button>
          <Dialog open={open} onOpenChange={(next) => (next ? setOpen(true) : close())} title={<span id={headingId}>{t('peerbonus.composer.title')}</span>}>
            {form}
          </Dialog>
        </>
      ) : null}
      {toast ? (
        <div className="pointer-events-none fixed inset-x-0 bottom-[calc(5.5rem+env(safe-area-inset-bottom))] z-40 flex justify-center px-4 md:bottom-6">
          <Notice tone="success" className="pointer-events-auto w-auto max-w-md shadow-lg animate-pop-in">
            {t('peerbonus.composer.sent', { name: toast.name, points: toast.points })}
          </Notice>
        </div>
      ) : null}
    </>
  )
}
