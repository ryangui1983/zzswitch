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
import { useMutation } from '@tanstack/react-query'
import { type Table } from '@tanstack/react-table'
import { Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { deleteUser } from '../api'
import { isUserDeleted } from '../constants'
import { type User } from '../types'
import { useUsers } from './users-provider'

interface DataTableBulkActionsProps {
  table: Table<User>
}

function canDeleteUser(user: User, currentRole: number | undefined): boolean {
  if (isUserDeleted(user)) return false
  if (currentRole == null) return false
  return user.role < currentRole
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const currentRole = useAuthStore((state) => state.auth.user?.role)
  const [deleteTargets, setDeleteTargets] = useState<User[] | null>(null)
  const selectedRows = table.getFilteredSelectedRowModel().rows
  const deletableUsers = selectedRows
    .map((row) => row.original)
    .filter((user) => canDeleteUser(user, currentRole))

  const deletion = useMutation({
    mutationFn: async (targets: User[]) => {
      const succeeded: User[] = []
      let lastError: unknown
      for (const user of targets) {
        try {
          const result = await deleteUser(user.id)
          if (!result.success) throw createServerError(result)
          succeeded.push(user)
        } catch (error) {
          lastError = error
          break
        }
      }
      if (succeeded.length === 0) {
        throw lastError ?? createServerError(undefined, t('Failed to delete user'))
      }
      return {
        count: succeeded.length,
        failedCount: targets.length - succeeded.length,
      }
    },
    onSuccess: ({ count, failedCount }) => {
      toast.success(t('Successfully deleted {{count}} users', { count }))
      if (failedCount > 0) {
        toast.error(t('Failed to delete {{count}} users', { count: failedCount }))
      }
      table.resetRowSelection()
      setDeleteTargets(null)
      triggerRefresh()
    },
    onError: (_error, targets) => {
      handleServerError(
        _error,
        t('Failed to delete {{count}} users', { count: targets.length })
      )
    },
  })

  return (
    <>
      <BulkActionsToolbar table={table} entityName='user'>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                className='size-8'
                aria-label={t('Delete selected users')}
                disabled={deletableUsers.length === 0 || deletion.isPending}
                onClick={() => setDeleteTargets(deletableUsers)}
              />
            }
          >
            <Trash2 aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('Delete selected users')}</TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>
      <ConfirmDialog
        destructive
        open={deleteTargets !== null}
        onOpenChange={(open) => {
          if (!open && !deletion.isPending) setDeleteTargets(null)
        }}
        title={t('Delete {{count}} users?', {
          count: deleteTargets?.length ?? 0,
        })}
        desc={t('This action cannot be undone.')}
        confirmText={deletion.isPending ? t('Deleting...') : t('Delete')}
        isLoading={deletion.isPending}
        disabled={!deleteTargets?.length}
        handleConfirm={() => {
          if (deleteTargets?.length && !deletion.isPending) {
            deletion.mutate(deleteTargets)
          }
        }}
      />
    </>
  )
}
