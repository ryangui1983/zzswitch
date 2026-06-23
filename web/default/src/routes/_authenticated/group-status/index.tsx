import { createFileRoute } from '@tanstack/react-router'

function GroupStatus() {
  return (
    <iframe
      src='https://monitor.zzswitch.com/status/status'
      style={{ width: '100%', height: 'calc(100vh - 64px)', border: 'none' }}
      title='分组状态'
    />
  )
}

export const Route = createFileRoute('/_authenticated/group-status/')({
  component: GroupStatus,
})
