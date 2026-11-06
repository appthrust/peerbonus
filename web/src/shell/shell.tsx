import { AppSwitcher } from '@appthrust/app-launcher'
import { HandCoins, House, Inbox, Languages, Settings } from 'lucide-react'
import type { ReactNode } from 'react'
import { setLanguage, useText } from '../i18n'
import { timeUntil } from '../model/period'
import { paths } from '../routes/paths'
import { Avatar, Button, CLOCK_TICK_MS, cn, HrefLink, Ring, useNow } from '../ui'
import { useSession } from './use-session'

const NAV_LINK =
  'flex min-h-10 min-w-10 items-center justify-center gap-1.5 rounded-md px-2.5 text-sm font-medium text-stone-600 hover:bg-stone-100 hover:text-stone-900 md:min-h-9 [&_svg]:size-4 [&_svg]:shrink-0'
const NAV_ACTIVE = { className: 'bg-accent-soft text-accent-strong hover:bg-accent-soft hover:text-accent-strong' }
// The sections, the balance, the language switch, the switcher and the signed-in member do not fit a phone's width;
// below `sm` the section links show their icon and keep the label for screen readers and as a tooltip.
const NAV_LABEL = 'max-sm:sr-only'

/**
 * Top bar (brand, タイムライン, 受信箱 with the unread count, 管理 for a
 * peerbonus admin, this week's balance as a ring, language, the app
 * switcher, the signed-in member) over the page. The balance lives only here
 * (the form shows what a send leaves); after a send the ring animates down.
 */
export function Shell({ children }: { children: ReactNode }) {
  const { t, language } = useText()
  const { me } = useSession()
  const { period } = me.stats
  const unread = me.inbox.unread
  // A ticking clock, so the reset text stays true on a page left open.
  const now = useNow(CLOCK_TICK_MS)
  const resets = timeUntil(period.endsAt, now)
  const resetText = resets.days > 0 ? t('peerbonus.common.resetsInDays', { days: resets.days }) : t('peerbonus.common.resetsInHours', { hours: resets.hours })
  return (
    <div className="min-h-dvh">
      <a
        href="#main"
        className="sr-only z-50 min-h-10 content-center rounded-md bg-white px-3 py-2 text-sm font-medium shadow focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
      >
        {t('peerbonus.nav.skipToMain')}
      </a>
      <header className="sticky top-0 z-10 border-b border-line bg-white/90 backdrop-blur supports-[backdrop-filter]:bg-white/80">
        <div className="mx-auto flex h-14 max-w-5xl items-center gap-1 px-3 sm:gap-2 sm:px-6">
          <HrefLink href={paths.home} className="mr-2 flex shrink-0 items-center gap-2.5 text-[15px] font-semibold tracking-tight text-stone-900" activeOptions={{ includeSearch: false }}>
            <span aria-hidden className="grid size-7 place-items-center rounded-md bg-accent text-white">
              <HandCoins className="size-4" />
            </span>
            <span className="hidden sm:inline">{t('peerbonus.product')}</span>
          </HrefLink>
          <nav aria-label={t('peerbonus.nav.label')} className="flex min-w-0 items-center gap-1">
            <HrefLink href={paths.home} title={t('peerbonus.nav.home')} className={NAV_LINK} activeProps={NAV_ACTIVE} activeOptions={{ includeSearch: false }}>
              <House aria-hidden />
              <span className={NAV_LABEL}>{t('peerbonus.nav.home')}</span>
            </HrefLink>
            <HrefLink href={paths.inbox} title={t('peerbonus.nav.inbox')} className={NAV_LINK} activeProps={NAV_ACTIVE} activeOptions={{ includeSearch: false }}>
              <Inbox aria-hidden />
              <span className={NAV_LABEL}>{t('peerbonus.nav.inbox')}</span>
              {unread > 0 ? (
                <>
                  <span aria-hidden className="-ml-0.5 min-w-5 rounded-full bg-accent px-1.5 text-center text-[11px] leading-5 font-semibold text-white tabular-nums">
                    {unread > 99 ? '99+' : unread}
                  </span>
                  <span className="sr-only">{t('peerbonus.nav.unread', { count: unread })}</span>
                </>
              ) : null}
            </HrefLink>
            {me.roles.peerbonusAdmin ? (
              <HrefLink href={paths.admin} title={t('peerbonus.nav.admin')} className={NAV_LINK} activeProps={NAV_ACTIVE} activeOptions={{ exact: false, includeSearch: false }}>
                <Settings aria-hidden />
                <span className={NAV_LABEL}>{t('peerbonus.nav.admin')}</span>
              </HrefLink>
            ) : null}
          </nav>
          <div className="ml-auto flex shrink-0 items-center gap-1 sm:gap-2">
            <HrefLink
              href={paths.me}
              title={resetText}
              className="flex min-h-10 items-center gap-1.5 rounded-full px-1.5 text-xs font-semibold text-stone-800 tabular-nums hover:bg-stone-100 sm:pr-2.5 md:min-h-9"
            >
              <Ring value={period.remaining} max={period.allowance} size={24} strokeWidth={3.5} />
              <span aria-hidden className="max-sm:hidden">
                {t('peerbonus.nav.balanceLeft', { remaining: period.remaining })}
              </span>
              <span aria-hidden className="sm:hidden">
                {period.remaining}
              </span>
              <span className="sr-only">
                {t('peerbonus.nav.balance', { remaining: period.remaining })}
                {' · '}
                {resetText}
              </span>
            </HrefLink>
            <Button
              tone="ghost"
              size="sm"
              icon={<Languages />}
              aria-label={`${t('peerbonus.nav.language')}: ${t(language === 'ja' ? 'peerbonus.language.en' : 'peerbonus.language.ja')}`}
              onClick={() => setLanguage(language === 'ja' ? 'en' : 'ja')}
            >
              <span lang={language === 'ja' ? 'en' : 'ja'} className="hidden sm:inline">
                {t(language === 'ja' ? 'peerbonus.language.en' : 'peerbonus.language.ja')}
              </span>
            </Button>
            <AppSwitcher language={language} />
            <HrefLink href={paths.member(me.member.id)} className={cn(NAV_LINK, 'pl-1.5')} activeProps={NAV_ACTIVE} activeOptions={{ includeSearch: false }}>
              <Avatar name={me.member.displayName} avatarUrl={me.member.avatarUrl} size="sm" />
              <span className="max-w-32 truncate max-md:sr-only">
                <span className="sr-only">{t('peerbonus.nav.you')}: </span>
                {me.member.displayName}
              </span>
            </HrefLink>
          </div>
        </div>
      </header>
      <main id="main" tabIndex={-1} className="mx-auto w-full max-w-5xl px-4 py-6 focus:outline-none sm:px-6 md:py-8">
        {children}
      </main>
    </div>
  )
}
