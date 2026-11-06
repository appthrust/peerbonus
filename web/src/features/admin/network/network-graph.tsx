import { useNavigate } from '@tanstack/react-router'
import { useId, useLayoutEffect, useMemo, useRef, useState, type MouseEvent, type PointerEvent, type ReactNode } from 'react'
import type { Network, NetworkNode } from '../../../api/peerbonus'
import { useText } from '../../../i18n'
import { paths } from '../../../routes/paths'
import { cn, initialsOf } from '../../../ui'
import { formatCount } from '../format'
import { layoutNetwork, nodeRadius, type Point } from './layout'
import { badgeOf, hasSignal, SIGNAL_GROUPS } from './signals'

/** Stroke width of an edge: thicker with more posts and claps between the pair, capped. */
const edgeWidth = (exchanges: number) => 1 + Math.min(exchanges, 12) * 0.35

/** Edge styles: this week (solid), earlier in the current month (faint, dashed), or a past month (one plain style). */
const EDGE_STYLE = {
  recent: { className: 'stroke-stone-500', opacity: 0.85, dash: undefined },
  earlier: { className: 'stroke-stone-400', opacity: 0.45, dash: '4 4' },
  month: { className: 'stroke-stone-400', opacity: 0.7, dash: undefined },
} as const

function LegendItem({ mark, children }: { mark: ReactNode; children: ReactNode }) {
  return (
    <li className="flex items-center gap-1.5">
      <span aria-hidden className="inline-flex shrink-0 items-center">
        {mark}
      </span>
      {children}
    </li>
  )
}

function LegendLine({ style }: { style: keyof typeof EDGE_STYLE }) {
  const edge = EDGE_STYLE[style]
  return (
    <svg width="24" height="8" viewBox="0 0 24 8">
      <line x1="1" y1="4" x2="23" y2="4" strokeWidth="2" strokeDasharray={edge.dash} strokeOpacity={edge.opacity} className={edge.className} />
    </svg>
  )
}

/** Above the graph: node sizes, the viewer's outline, the four signal colours and the edge styles. */
function Legend({ data }: { data: Network }) {
  const { t } = useText()
  const many = data.thresholds.manyConnections
  return (
    <div className="mb-3">
      <h3 className="sr-only">{t('peerbonus.network.legend')}</h3>
      <ul className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-stone-600">
        <LegendItem
          mark={
            <svg width="30" height="20" viewBox="0 0 30 20">
              <circle cx="6" cy="14" r="5" className="fill-accent-soft stroke-stone-300" />
              <circle cx="20" cy="10" r="9" className="fill-accent-soft stroke-stone-300" />
            </svg>
          }
        >
          {t('peerbonus.network.legend.size')}
        </LegendItem>
        <LegendItem mark={<span className="size-4 rounded-full border-[3px] border-stone-400 bg-accent-soft" />}>{t('peerbonus.network.legend.hub', { many })}</LegendItem>
        <LegendItem mark={<span className="size-4 rounded-full bg-accent-soft ring-[3px] ring-accent" />}>{t('peerbonus.nav.you')}</LegendItem>
        {SIGNAL_GROUPS.map((group) => (
          <LegendItem key={group.key} mark={<span className={cn('size-2.5 rounded-full ring-2 ring-white', group.dot)} />}>
            {t(group.label)}
          </LegendItem>
        ))}
        {data.recentSince ? (
          <>
            <LegendItem mark={<LegendLine style="recent" />}>{t('peerbonus.network.legend.thisWeek')}</LegendItem>
            <LegendItem mark={<LegendLine style="earlier" />}>{t('peerbonus.network.legend.earlier')}</LegendItem>
          </>
        ) : (
          <LegendItem mark={<LegendLine style="month" />}>{t('peerbonus.network.legend.exchange')}</LegendItem>
        )}
      </ul>
    </div>
  )
}

/** The facts of the hovered or focused person, above (or, near the top, below) their node. */
function NodeTip({ id, node, at, radius, extent }: { id: string; node: NetworkNode; at: Point; radius: number; extent: number }) {
  const { t, locale } = useText()
  const below = at.y - radius < -extent + 130
  const groups = SIGNAL_GROUPS.filter((group) => hasSignal(node, group.key))
  return (
    <div
      id={id}
      role="tooltip"
      className={cn(
        'pointer-events-none absolute z-10 w-max max-w-60 -translate-x-1/2 rounded-md bg-stone-900 px-3 py-2 text-xs text-white shadow-lg',
        below ? 'translate-y-2' : '-translate-y-full'
      )}
      style={{ left: extent + at.x, top: extent + at.y + (below ? radius : -radius - 8) }}
    >
      <p className="font-semibold">{node.displayName}</p>
      <p className="mt-0.5 text-stone-300">{t('peerbonus.network.tip.connections', { count: node.connections })}</p>
      <p className="text-stone-300">{t('peerbonus.network.tip.received', { posts: node.receivedPosts, points: formatCount(node.receivedPoints, locale) })}</p>
      <p className="text-stone-300">{t('peerbonus.network.tip.sent', { posts: node.sentPosts })}</p>
      <p className="text-stone-300">{t('peerbonus.network.tip.claps', { given: node.clapsGiven, received: node.clapsReceived })}</p>
      {groups.length > 0 || node.signals.includes('manyConnections') ? (
        <ul className="mt-1.5 flex flex-wrap gap-x-2 gap-y-1">
          {node.signals.includes('manyConnections') ? <li className="font-medium">{t('peerbonus.network.signal.hub')}</li> : null}
          {groups.map((group) => (
            <li key={group.key} className="flex items-center gap-1 font-medium">
              <span aria-hidden className={cn('size-2 rounded-full', group.dot)} />
              {t(group.label)}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

/**
 * The connection graph: an SVG at natural size (1 unit = 1 px) inside a fixed-height box that pans by
 * scrolling (wheel, touch) or by dragging the background. Nodes are focusable; hover or focus shows the
 * person's facts and highlights their connections; a member's node opens their page.
 */
export function NetworkGraph({ data, viewer }: { data: Network; viewer: string }) {
  const { t } = useText()
  const navigate = useNavigate()
  const uid = useId()
  const many = data.thresholds.manyConnections
  const layout = useMemo(
    () =>
      layoutNetwork(
        data.nodes.map((node) => ({ id: node.personId, connections: node.connections })),
        data.edges,
        many
      ),
    [data.nodes, data.edges, many]
  )
  const [active, setActive] = useState<string | null>(null)
  const [focusRing, setFocusRing] = useState<string | null>(null)
  const [failed, setFailed] = useState<ReadonlySet<string>>(() => new Set())
  const [dragging, setDragging] = useState(false)
  const scroller = useRef<HTMLDivElement>(null)
  const drag = useRef<{ x: number; y: number; left: number; top: number } | null>(null)
  const size = layout.extent * 2
  const clipId = `${uid}-clip`
  const tipId = `${uid}-tip`
  const recentSince = data.recentSince ? Date.parse(data.recentSince) : null

  // Start with the middle of the drawing in view.
  useLayoutEffect(() => {
    const box = scroller.current
    if (!box) return
    box.scrollLeft = (box.scrollWidth - box.clientWidth) / 2
    box.scrollTop = (box.scrollHeight - box.clientHeight) / 2
  }, [layout])

  const neighbours = useMemo(() => {
    if (!active) return null
    const ids = new Set([active])
    for (const edge of data.edges) {
      if (edge.from === active) ids.add(edge.to)
      if (edge.to === active) ids.add(edge.from)
    }
    return ids
  }, [active, data.edges])

  const startDrag = (event: PointerEvent<HTMLDivElement>) => {
    const box = scroller.current
    if (!box || event.pointerType !== 'mouse' || event.button !== 0 || (event.target as Element).closest('[data-node]')) return
    drag.current = { x: event.clientX, y: event.clientY, left: box.scrollLeft, top: box.scrollTop }
    event.currentTarget.setPointerCapture(event.pointerId)
    setDragging(true)
  }
  const moveDrag = (event: PointerEvent<HTMLDivElement>) => {
    const box = scroller.current
    if (!box || !drag.current) return
    box.scrollLeft = drag.current.left - (event.clientX - drag.current.x)
    box.scrollTop = drag.current.top - (event.clientY - drag.current.y)
  }
  const endDrag = () => {
    drag.current = null
    setDragging(false)
  }
  const open = (event: MouseEvent, memberId: string) => {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) return
    event.preventDefault()
    void navigate({ to: '/members/$memberId', params: { memberId } })
  }

  const activeNode = active ? data.nodes.find((node) => node.personId === active) : undefined
  const activeAt = active ? layout.positions.get(active) : undefined

  return (
    <div>
      <Legend data={data} />
      <div
        ref={scroller}
        onPointerDown={startDrag}
        onPointerMove={moveDrag}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        // A fixed-height window onto larger drawings; a small one (a narrow group) needs less.
        style={{ height: size + 2 }}
        className={cn(
          'max-h-[26rem] overflow-auto overscroll-contain rounded-lg border border-line bg-white select-none sm:max-h-[32rem] lg:max-h-[38rem]',
          dragging ? 'cursor-grabbing' : 'cursor-grab'
        )}
      >
        <div className="flex min-h-full w-max min-w-full items-center justify-center">
          <div className="relative shrink-0" style={{ width: size, height: size }}>
            <svg
              width={size}
              height={size}
              viewBox={`${-layout.extent} ${-layout.extent} ${size} ${size}`}
              role="group"
              aria-label={t('peerbonus.network.graphLabel', { count: data.nodes.length })}
              className="block"
            >
              <defs>
                <clipPath id={clipId} clipPathUnits="objectBoundingBox">
                  <circle cx="0.5" cy="0.5" r="0.5" />
                </clipPath>
              </defs>
              <g aria-hidden>
                {data.edges.map((edge) => {
                  const from = layout.positions.get(edge.from)
                  const to = layout.positions.get(edge.to)
                  if (!from || !to) return null
                  const style = EDGE_STYLE[recentSince === null ? 'month' : Date.parse(edge.lastAt) >= recentSince ? 'recent' : 'earlier']
                  const lit = neighbours !== null && (edge.from === active || edge.to === active)
                  return (
                    <line
                      key={`${edge.from}|${edge.to}`}
                      x1={from.x}
                      y1={from.y}
                      x2={to.x}
                      y2={to.y}
                      strokeWidth={edgeWidth(edge.posts + edge.claps)}
                      strokeDasharray={lit ? undefined : style.dash}
                      strokeOpacity={lit ? 1 : neighbours ? 0.12 : style.opacity}
                      strokeLinecap="round"
                      className={lit ? 'stroke-accent' : style.className}
                    />
                  )
                })}
              </g>
              {data.nodes.map((node) => {
                const at = layout.positions.get(node.personId)
                if (!at) return null
                const r = nodeRadius(node.connections, many)
                const badge = badgeOf(node)
                const self = node.personId === viewer
                const hub = node.connections >= many
                const picture = node.avatarUrl && !failed.has(node.personId) ? node.avatarUrl : null
                const content = (
                  <g transform={`translate(${at.x} ${at.y})`} opacity={neighbours && !neighbours.has(node.personId) ? 0.35 : 1}>
                    {focusRing === node.personId ? <circle r={r + 5} fill="none" strokeWidth="2" className="stroke-accent" /> : null}
                    {picture ? (
                      <>
                        <circle r={r} className="fill-stone-200" />
                        <image
                          href={picture}
                          x={-r}
                          y={-r}
                          width={r * 2}
                          height={r * 2}
                          preserveAspectRatio="xMidYMid slice"
                          clipPath={`url(#${clipId})`}
                          onError={() => setFailed((previous) => new Set(previous).add(node.personId))}
                        />
                      </>
                    ) : (
                      <>
                        <circle r={r} className="fill-accent-soft" />
                        <text textAnchor="middle" dominantBaseline="central" fontSize={Math.max(11, Math.round(r * 0.8))} className="fill-accent-strong font-semibold">
                          {initialsOf(node.displayName)}
                        </text>
                      </>
                    )}
                    <circle r={r} fill="none" strokeWidth={self ? 3.5 : hub ? 3 : 1.5} className={self ? 'stroke-accent' : hub ? 'stroke-stone-400' : 'stroke-stone-300'} />
                    {badge ? <circle cx={r * 0.72} cy={-r * 0.72} r="5.5" strokeWidth="2" className={cn('stroke-white', badge.fill)} /> : null}
                  </g>
                )
                const common = {
                  'data-node': node.personId,
                  'aria-label': t('peerbonus.network.nodeLabel', { name: node.displayName, count: node.connections }),
                  'aria-describedby': active === node.personId ? tipId : undefined,
                  className: 'cursor-pointer outline-none',
                  onPointerEnter: () => setActive(node.personId),
                  onPointerLeave: () => setActive((current) => (current === node.personId ? null : current)),
                  onFocus: (event: { currentTarget: Element }) => {
                    setActive(node.personId)
                    setFocusRing(event.currentTarget.matches(':focus-visible') ? node.personId : null)
                  },
                  onBlur: () => {
                    setActive((current) => (current === node.personId ? null : current))
                    setFocusRing(null)
                  },
                }
                return node.memberId ? (
                  <a key={node.personId} href={paths.member(node.memberId)} onClick={(event) => open(event, node.memberId!)} {...common}>
                    {content}
                  </a>
                ) : (
                  <g key={node.personId} role="img" tabIndex={0} {...common} className="cursor-default outline-none">
                    {content}
                  </g>
                )
              })}
            </svg>
            {activeNode && activeAt ? <NodeTip id={tipId} node={activeNode} at={activeAt} radius={nodeRadius(activeNode.connections, many)} extent={layout.extent} /> : null}
          </div>
        </div>
      </div>
    </div>
  )
}
