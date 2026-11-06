// Deterministic force-directed layout of the network map (design "v3: network
// map" → Screen): no layout dependency, nodes ≤ 500. People with connections
// run a Fruchterman–Reingold simulation (repulsion between every pair, springs
// on edges, a pull to the centre, a cooling step limit), seeded from the
// sorted person ids so the same month draws the same picture on every load;
// people with no connection sit on rings outside it, visible as a group.

/** `connections`: distinct counterparts Project-wide (a node with none is isolated). */
export type LayoutNode = { id: string; connections: number }
export type LayoutEdge = { from: string; to: string }
export type Point = { x: number; y: number }
/**
 * Positions are centred on (0, 0). `mainRadius`: every connected node's circle lies within it.
 * `ringRadius`: the innermost ring of isolated nodes (null without any). `extent`: half the side of
 * the square that holds everything, badges and margin included.
 */
export type GraphLayout = { positions: Map<string, Point>; mainRadius: number; ringRadius: number | null; extent: number }

/** Distance an edge pulls towards, in px. */
const IDEAL = 80
const ITERATIONS = 150
/** Passes that push overlapping circles apart once the simulation has cooled. */
const COLLISION_PASSES = 60
/** Pull towards the centre per px of distance: keeps unlinked parts of a filtered view close. */
const GRAVITY = 0.06
/** Gap kept between two node circles, and between rings. */
const GAP = 10
const MARGIN = 28
const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5))

/**
 * Node radius in px: 14 for nobody, growing to 22 just under `many`; hubs (`connections >= many`)
 * jump to 26 and grow to 34, so they read clearly larger.
 */
export function nodeRadius(connections: number, many: number): number {
  if (connections >= many) return 26 + Math.min(connections - many, 8)
  return 14 + (8 * Math.max(connections, 0)) / Math.max(many, 1)
}

/** FNV-1a over the ids, the PRNG seed. */
function hashIds(ids: readonly string[]): number {
  let hash = 0x811c9dc5
  for (const id of ids) {
    for (let index = 0; index < id.length; index++) hash = Math.imul(hash ^ id.charCodeAt(index), 0x01000193)
    hash = Math.imul(hash ^ 0x2c, 0x01000193)
  }
  return hash >>> 0
}

/** mulberry32. */
function seededRandom(seed: number): () => number {
  let a = seed
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** Lays out `nodes` (any order: the result depends only on the ids, connections and edges). */
export function layoutNetwork(nodes: readonly LayoutNode[], edges: readonly LayoutEdge[], many: number): GraphLayout {
  const sorted = [...nodes].sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
  const rand = seededRandom(hashIds(sorted.map((node) => node.id)))
  const linked = sorted.filter((node) => node.connections > 0)
  const isolated = sorted.filter((node) => node.connections <= 0)
  const count = linked.length
  const index = new Map(linked.map((node, position) => [node.id, position]))
  const radius = linked.map((node) => nodeRadius(node.connections, many))
  const springs = edges
    .map((edge) => [index.get(edge.from), index.get(edge.to)] as const)
    .filter((pair): pair is readonly [number, number] => pair[0] !== undefined && pair[1] !== undefined && pair[0] !== pair[1])
    .sort((a, b) => a[0] - b[0] || a[1] - b[1])

  // Start on a sunflower spiral (even density), jittered by the seed.
  const x = new Float64Array(count)
  const y = new Float64Array(count)
  for (let i = 0; i < count; i++) {
    const angle = i * GOLDEN_ANGLE + rand() * 0.6
    const distance = IDEAL * 0.6 * Math.sqrt(i + 0.5)
    x[i] = Math.cos(angle) * distance
    y[i] = Math.sin(angle) * distance
  }
  const dx = new Float64Array(count)
  const dy = new Float64Array(count)
  const start = IDEAL * 1.5
  for (let step = 0; step < ITERATIONS; step++) {
    dx.fill(0)
    dy.fill(0)
    for (let i = 0; i < count; i++) {
      for (let j = i + 1; j < count; j++) {
        let ux = x[i]! - x[j]!
        let uy = y[i]! - y[j]!
        let distance = Math.hypot(ux, uy)
        if (distance < 0.01) {
          ux = rand() - 0.5
          uy = rand() - 0.5
          distance = Math.hypot(ux, uy)
        }
        const room = radius[i]! + radius[j]! + GAP
        // Repulsion, plus a hard push while two circles overlap.
        const force = (IDEAL * IDEAL) / distance + (distance < room ? (room - distance) * 2 : 0)
        const fx = (ux / distance) * force
        const fy = (uy / distance) * force
        dx[i]! += fx
        dy[i]! += fy
        dx[j]! -= fx
        dy[j]! -= fy
      }
    }
    for (const [i, j] of springs) {
      const ux = x[i]! - x[j]!
      const uy = y[i]! - y[j]!
      const distance = Math.hypot(ux, uy) || 0.01
      const force = (distance * distance) / IDEAL
      const fx = (ux / distance) * force
      const fy = (uy / distance) * force
      dx[i]! -= fx
      dy[i]! -= fy
      dx[j]! += fx
      dy[j]! += fy
    }
    const limit = start * (1 - step / ITERATIONS) + 1
    for (let i = 0; i < count; i++) {
      const mx = dx[i]! - x[i]! * GRAVITY * IDEAL
      const my = dy[i]! - y[i]! * GRAVITY * IDEAL
      const length = Math.hypot(mx, my)
      if (length === 0) continue
      const move = Math.min(length, limit)
      x[i]! += (mx / length) * move
      y[i]! += (my / length) * move
    }
  }
  // The springs can still press circles together; separate them pairwise until none overlap.
  for (let pass = 0; pass < COLLISION_PASSES; pass++) {
    let moved = false
    for (let i = 0; i < count; i++) {
      for (let j = i + 1; j < count; j++) {
        const ux = x[i]! - x[j]!
        const uy = y[i]! - y[j]!
        const distance = Math.hypot(ux, uy) || 0.01
        const room = radius[i]! + radius[j]! + GAP
        if (distance >= room) continue
        const push = (room - distance) / 2
        x[i]! += (ux / distance) * push
        y[i]! += (uy / distance) * push
        x[j]! -= (ux / distance) * push
        y[j]! -= (uy / distance) * push
        moved = true
      }
    }
    if (!moved) break
  }

  let cx = 0
  let cy = 0
  for (let i = 0; i < count; i++) {
    cx += x[i]! / count
    cy += y[i]! / count
  }
  const positions = new Map<string, Point>()
  let mainRadius = 0
  for (const [i, node] of linked.entries()) {
    const point = { x: x[i]! - cx, y: y[i]! - cy }
    positions.set(node.id, point)
    mainRadius = Math.max(mainRadius, Math.hypot(point.x, point.y) + radius[i]!)
  }

  // Isolated people: rings outside the connected part, filled from the top, clockwise.
  const small = nodeRadius(0, many)
  const slot = 2 * small + GAP
  let ringRadius: number | null = null
  let outer = mainRadius
  let placed = 0
  for (let ring = 0; placed < isolated.length; ring++) {
    const distance = (count > 0 ? mainRadius + 2 * GAP + small : isolated.length === 1 ? 0 : slot) + ring * slot
    const capacity = distance === 0 ? 1 : Math.max(1, Math.floor((2 * Math.PI * distance) / slot))
    const here = Math.min(capacity, isolated.length - placed)
    const offset = ring % 2 === 0 ? 0 : Math.PI / here
    for (let k = 0; k < here; k++) {
      const angle = -Math.PI / 2 + offset + (2 * Math.PI * k) / here
      positions.set(isolated[placed + k]!.id, { x: Math.cos(angle) * distance, y: Math.sin(angle) * distance })
    }
    ringRadius ??= distance
    outer = distance + small
    placed += here
  }
  return { positions, mainRadius, ringRadius, extent: Math.ceil(outer + MARGIN) }
}
