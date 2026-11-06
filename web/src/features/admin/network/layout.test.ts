import { describe, expect, test } from 'bun:test'
import { layoutNetwork, nodeRadius, type LayoutEdge, type LayoutNode } from './layout'

const MANY = 8

/** Two clusters joined by a hub, a chain, and four people with no connection. */
const NODES: LayoutNode[] = [
  { id: 'hub', connections: 9 },
  ...Array.from({ length: 6 }, (_, index) => ({ id: `a${index}`, connections: 3 })),
  ...Array.from({ length: 5 }, (_, index) => ({ id: `b${index}`, connections: 2 })),
  { id: 'chain', connections: 1 },
  ...Array.from({ length: 4 }, (_, index) => ({ id: `quiet${index}`, connections: 0 })),
]
const EDGES: LayoutEdge[] = [
  ...Array.from({ length: 6 }, (_, index) => ({ from: `a${index}`, to: 'hub' })),
  ...Array.from({ length: 5 }, (_, index) => ({ from: `b${index}`, to: 'hub' })),
  { from: 'a0', to: 'a1' },
  { from: 'a2', to: 'a3' },
  { from: 'b0', to: 'b1' },
  { from: 'b4', to: 'chain' },
]

const distance = (point: { x: number; y: number }) => Math.hypot(point.x, point.y)

describe('network layout', () => {
  test('the same input draws the same picture, whatever order the nodes and edges arrive in', () => {
    const first = layoutNetwork(NODES, EDGES, MANY)
    const again = layoutNetwork(NODES, EDGES, MANY)
    const shuffled = layoutNetwork([...NODES].reverse(), [...EDGES].reverse(), MANY)
    expect([...again.positions]).toEqual([...first.positions])
    for (const node of NODES) expect(shuffled.positions.get(node.id)).toEqual(first.positions.get(node.id)!)
    expect(shuffled.extent).toBe(first.extent)
  })

  test('isolated people sit on a ring outside the connected part', () => {
    const layout = layoutNetwork(NODES, EDGES, MANY)
    const small = nodeRadius(0, MANY)
    expect(layout.ringRadius).not.toBeNull()
    expect(layout.ringRadius!).toBeGreaterThan(layout.mainRadius)
    for (const node of NODES) {
      const at = layout.positions.get(node.id)!
      if (node.connections === 0) expect(distance(at) - small).toBeGreaterThan(layout.mainRadius)
      else expect(distance(at) + nodeRadius(node.connections, MANY)).toBeLessThanOrEqual(layout.mainRadius + 1e-9)
      expect(distance(at) + nodeRadius(node.connections, MANY)).toBeLessThanOrEqual(layout.extent)
    }
  })

  test('no two circles overlap', () => {
    const layout = layoutNetwork(NODES, EDGES, MANY)
    for (const [index, a] of NODES.entries()) {
      for (const b of NODES.slice(index + 1)) {
        const pa = layout.positions.get(a.id)!
        const pb = layout.positions.get(b.id)!
        expect(Math.hypot(pa.x - pb.x, pa.y - pb.y)).toBeGreaterThanOrEqual(nodeRadius(a.connections, MANY) + nodeRadius(b.connections, MANY))
      }
    }
  })

  test('hubs are clearly larger than anyone below the threshold', () => {
    expect(nodeRadius(MANY, MANY) - nodeRadius(MANY - 1, MANY)).toBeGreaterThanOrEqual(4)
    expect(nodeRadius(0, MANY)).toBeLessThan(nodeRadius(2, MANY))
  })

  test('a view where nobody is connected is only rings around the centre', () => {
    const layout = layoutNetwork([{ id: 'x', connections: 0 }, { id: 'y', connections: 0 }], [], MANY)
    expect(layout.mainRadius).toBe(0)
    expect(distance(layout.positions.get('x')!)).toBeCloseTo(layout.ringRadius!)
    expect(distance(layoutNetwork([{ id: 'solo', connections: 0 }], [], MANY).positions.get('solo')!)).toBe(0)
  })
})
