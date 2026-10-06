/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toaster, toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { useDataTable } from '@/components/data-table'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { getUsers } from '../../api'
import type { User } from '../../types'
import { DataTableBulkActions } from '../data-table-bulk-actions'
import { UsersProvider, useUsers } from '../users-provider'

const users: User[] = [11, 22, 33].map((id) => ({
  id,
  username: `user-${id}`,
  display_name: `User ${id}`,
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
}))
const columns = [{ accessorKey: 'username' }]
const clients: QueryClient[] = []

function UserList() {
  const { refreshTrigger } = useUsers()
  const { data } = useQuery({
    queryKey: ['users', refreshTrigger],
    queryFn: () => getUsers(),
  })
  const { table } = useDataTable({
    data: data?.data?.items ?? [],
    columns,
    enableRowSelection: true,
    getRowId: (row) => String(row.id),
  })
  return (
    <>
      {table.getRowModel().rows.map((row) => (
        <label key={row.id}>
          <input
            type='checkbox'
            checked={row.getIsSelected()}
            onChange={row.getToggleSelectedHandler()}
          />
          {row.original.username}
        </label>
      ))}
      <DataTableBulkActions table={table} />
    </>
  )
}

async function setup() {
  let remaining = [...users]
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: {
      success: true,
      data: { items: remaining, total: remaining.length },
    },
  }))
  const remove = vi.spyOn(api, 'delete').mockImplementation(async (url) => {
    const id = Number(String(url).match(/\/api\/user\/(\d+)\//)?.[1])
    remaining = remaining.filter((user) => user.id !== id)
    return { data: { success: true } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UserList />
      </UsersProvider>
      <Toaster />
    </QueryClientProvider>
  )
  await screen.findByRole('checkbox', { name: 'user-11' })
  return { user: userEvent.setup(), remove }
}

afterEach(() => {
  toast.dismiss()
  for (const client of clients) client.clear()
  clients.length = 0
  useAuthStore.getState().auth.reset()
})

test('shows deletion only for selected users and cancellation sends no requests', async () => {
  const { user, remove } = await setup()
  expect(
    screen.queryByRole('button', { name: 'Delete selected users' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('checkbox', { name: 'user-11' }))
  const button = screen.getByRole('button', {
    name: 'Delete selected users',
  })
  button.focus()
  await user.keyboard('{Enter}')
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog).toHaveAccessibleName('Delete 1 users?')
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(remove).not.toHaveBeenCalled()
  expect(screen.getByRole('checkbox', { name: 'user-11' })).toBeChecked()
})

test('confirmation deletes only selected users, disables repeat submissions and refreshes the list', async () => {
  const { user, remove } = await setup()
  let release!: () => void
  const pending = new Promise<void>((resolve) => {
    release = resolve
  })
  const originalDelete = remove.getMockImplementation()
  if (!originalDelete) throw new Error('Missing delete mock')
  remove.mockImplementationOnce(async (url) => {
    await pending
    return originalDelete(url)
  })
  await user.click(screen.getByRole('checkbox', { name: 'user-11' }))
  await user.click(screen.getByRole('checkbox', { name: 'user-22' }))
  await user.click(
    screen.getByRole('button', { name: 'Delete selected users' })
  )
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog).toHaveAccessibleName('Delete 2 users?')
  expect(remove).not.toHaveBeenCalled()
  await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
  expect(
    within(dialog).getByRole('button', { name: 'Deleting...' })
  ).toBeDisabled()
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
  release()
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('checkbox', { name: 'user-11' })
    ).not.toBeInTheDocument()
  )
  expect(remove.mock.calls.map(([url]) => url)).toEqual([
    '/api/user/11/',
    '/api/user/22/',
  ])
  expect(screen.getByRole('checkbox', { name: 'user-33' })).not.toBeChecked()
  expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
  expect(document.body).toHaveTextContent('Successfully deleted 2 users')
})

test.each(['server', 'network'])(
  '%s failure keeps selected users and retries the failed deletes',
  async (failure) => {
    const { user, remove } = await setup()
    if (failure === 'server') {
      remove.mockResolvedValueOnce({ data: { success: false } })
    } else {
      remove.mockRejectedValueOnce(new Error('Network unavailable'))
    }
    await user.click(screen.getByRole('checkbox', { name: 'user-11' }))
    await user.click(screen.getByRole('checkbox', { name: 'user-22' }))
    await user.click(
      screen.getByRole('button', { name: 'Delete selected users' })
    )
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
    await waitFor(() =>
      expect(document.body).toHaveTextContent(
        failure === 'server'
          ? 'Failed to delete 2 users'
          : 'Network unavailable'
      )
    )
    expect(dialog).toHaveAccessibleName('Delete 2 users?')
    expect(screen.getByLabelText('user-11')).toBeChecked()
    expect(screen.getByLabelText('user-22')).toBeChecked()
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    )
    expect(screen.queryByRole('toolbar')).not.toBeInTheDocument()
  }
)
