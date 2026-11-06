import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Trash2 } from 'lucide-react'
import { useState } from 'react'
import { peerbonusApi, type Post } from '../../api/peerbonus'
import { invalidatePeerbonus, postQuery } from '../../api/queries'
import { useText } from '../../i18n'
import { useSession } from '../../shell/use-session'
import { ConfirmDialog, Menu } from '../../ui'

/**
 * The post's overflow menu (admins only; nothing renders for anyone else).
 * 投稿を削除 asks first (spec §3.6): the post leaves every timeline; while its
 * month is open the server reverses every point it and its claps moved. On the
 * post page (`leave`) the admin then lands on the timeline; in a list the post
 * simply drops out.
 */
export function PostMenu({ post, leave = false }: { post: Post; leave?: boolean }) {
  const { me } = useSession()
  if (!me.roles.peerbonusAdmin) return null
  return <AdminPostMenu post={post} leave={leave} />
}

function AdminPostMenu({ post, leave }: { post: Post; leave: boolean }) {
  const { t } = useText()
  const client = useQueryClient()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const mutation = useMutation({
    mutationFn: () => peerbonusApi.posts.remove(post.id),
    onSuccess: async () => {
      // Leave the page first: refetching the open post now would answer 404.
      if (leave) await navigate({ to: '/', replace: true })
      else setOpen(false)
      client.removeQueries({ queryKey: postQuery(post.id).queryKey })
      await invalidatePeerbonus(client)
    },
  })
  return (
    <>
      <Menu label={t('peerbonus.post.actions')} items={[{ label: t('peerbonus.post.delete'), icon: <Trash2 />, tone: 'danger', onSelect: () => setOpen(true) }]} />
      <ConfirmDialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) mutation.reset()
        }}
        title={t('peerbonus.post.deleteTitle')}
        body={t('peerbonus.post.deleteBody')}
        confirmLabel={t('peerbonus.post.delete')}
        tone="danger"
        onConfirm={() => mutation.mutate()}
        pending={mutation.isPending}
        error={mutation.error}
      />
    </>
  )
}
