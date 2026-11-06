// All Peer Bonus wording, as dashboard-i18n catalogs: the English source is the
// `defaultMessage`, Japanese (the audience's default) is `translations.ja`.
// Screens read it through `useText().t(id, values)` (src/i18n.tsx); no screen
// hardcodes user-facing text.
import type { DashboardMessageCatalog } from '@appthrust/platform-dashboard-i18n'
import { adminMessages } from './admin'
import { commonMessages } from './common'
import { composeMessages } from './compose'
import { inboxMessages } from './inbox'
import { memberMessages } from './members'
import { networkMessages } from './network'
import { postMessages } from './posts'
import { slackMessages } from './slack'

export const catalogs: readonly DashboardMessageCatalog[] = [commonMessages, postMessages, composeMessages, inboxMessages, memberMessages, adminMessages, networkMessages, slackMessages]

export type MessageId =
  | keyof typeof commonMessages
  | keyof typeof postMessages
  | keyof typeof composeMessages
  | keyof typeof inboxMessages
  | keyof typeof memberMessages
  | keyof typeof adminMessages
  | keyof typeof networkMessages
  | keyof typeof slackMessages
