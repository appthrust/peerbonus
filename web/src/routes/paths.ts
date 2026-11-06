// Every Peer Bonus URL. `/posts/<id>` is the post URL people share; `/me`
// forwards to the signed-in member's own page. `/admin/*` pages are admin-only.

/** A member page tab: the posts they received or sent. */
export type MemberTab = 'received' | 'sent'

const id = encodeURIComponent

export const paths = {
  home: '/',
  /** The timeline narrowed by value key, month (`YYYY-MM`), text and `mine`; no parameters: all posts. */
  timeline: (params: { value?: string; month?: string; q?: string; mine?: boolean } = {}) => {
    const search = new URLSearchParams()
    if (params.value) search.set('value', params.value)
    if (params.month) search.set('month', params.month)
    if (params.q) search.set('q', params.q)
    if (params.mine) search.set('mine', '1')
    const text = search.toString()
    return text ? `/?${text}` : '/'
  },
  /** The timeline with the send form open (empty states' 「贈る」). */
  compose: '/?compose=1',
  /** The send form with this Directory person as the recipient (「贈る」 on posts and member pages). */
  sendTo: (personId: string) => `/?to=${id(personId)}`,
  post: (postId: string) => `/posts/${id(postId)}`,
  member: (memberId: string, tab: MemberTab = 'received') => `/members/${id(memberId)}${tab === 'sent' ? '?tab=sent' : ''}`,
  me: '/me',
  /** Posts to the caller and claps on their posts. */
  inbox: '/inbox',
  admin: '/admin',
  adminProgram: '/admin/program',
  adminStatements: '/admin/statements',
  adminStatement: (month: string) => `/admin/statements/${id(month)}`,
  adminMembers: '/admin/members',
  adminInsights: '/admin/insights',
  /** Who exchanged posts and claps with whom (`?month=&group=`). */
  adminNetwork: '/admin/network',
  /** Slack connection, setup guide, delivery settings and log. */
  adminSlack: '/admin/slack',
}
