import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import apiClient from '../api/client'
import { TableSkeleton } from '../components/LoadingSkeleton'
import type { Guard, Site } from '../types'
import { UserPlus, Building2, Save } from 'lucide-react'

export default function Settings() {
  const qc = useQueryClient()
  const [activeTab, setActiveTab] = useState<'guards' | 'sites'>('guards')
  const [showAddGuard, setShowAddGuard] = useState(false)
  const [newGuard, setNewGuard] = useState({ name: '', employee_id: '', role: 'guard' })
  const [formError, setFormError] = useState('')

  const { data: guards, isLoading: guardsLoading } = useQuery<Guard[]>({
    queryKey: ['guards'],
    queryFn: () => apiClient.get('/guards').then(r => r.data),
  })

  const { data: sites, isLoading: sitesLoading } = useQuery<Site[]>({
    queryKey: ['sites'],
    queryFn: () => apiClient.get('/sites').then(r => r.data),
  })

  const createGuard = useMutation({
    mutationFn: (body: typeof newGuard) => apiClient.post('/guards', body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['guards'] })
      setShowAddGuard(false)
      setNewGuard({ name: '', employee_id: '', role: 'guard' })
      setFormError('')
    },
    onError: (err: any) => setFormError(err?.response?.data?.error ?? 'Failed to create guard'),
  })

  const updateGuardStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      apiClient.put(`/guards/${id}`, { status }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['guards'] }),
  })

  const tabs = [
    { id: 'guards' as const, label: 'Guards', icon: UserPlus },
    { id: 'sites' as const, label: 'Sites', icon: Building2 },
  ]

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold text-gray-800">Settings</h2>

      {/* Tabs */}
      <div className="flex border-b border-gray-200">
        {tabs.map(({ id, label, icon: Icon }) => (
          <button key={id} onClick={() => setActiveTab(id)}
            className={`flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
              activeTab === id
                ? 'border-blue-500 text-blue-600'
                : 'border-transparent text-gray-500 hover:text-gray-700'
            }`}>
            <Icon className="w-4 h-4" />{label}
          </button>
        ))}
      </div>

      {/* Guards tab */}
      {activeTab === 'guards' && (
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <p className="text-sm text-gray-500">{guards?.length ?? 0} guards registered</p>
            <button onClick={() => setShowAddGuard(!showAddGuard)}
              className="flex items-center gap-2 bg-blue-600 text-white px-3 py-2 rounded-lg text-sm hover:bg-blue-700 transition-colors">
              <UserPlus className="w-4 h-4" /> Add Guard
            </button>
          </div>

          {showAddGuard && (
            <div className="bg-blue-50 rounded-xl border border-blue-100 p-4 space-y-3">
              <h3 className="font-medium text-gray-800">New Guard</h3>
              {formError && <p className="text-sm text-red-600">{formError}</p>}
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <input value={newGuard.name} onChange={e => setNewGuard(g => ({ ...g, name: e.target.value }))}
                  placeholder="Full name *"
                  className="border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
                <input value={newGuard.employee_id} onChange={e => setNewGuard(g => ({ ...g, employee_id: e.target.value }))}
                  placeholder="Employee ID"
                  className="border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
                <select value={newGuard.role} onChange={e => setNewGuard(g => ({ ...g, role: e.target.value }))}
                  className="border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
                  <option value="guard">Guard</option>
                  <option value="supervisor">Supervisor</option>
                </select>
              </div>
              <div className="flex gap-2">
                <button onClick={() => createGuard.mutate(newGuard)}
                  disabled={!newGuard.name || createGuard.isPending}
                  className="flex items-center gap-1 bg-blue-600 text-white px-3 py-2 rounded-lg text-sm hover:bg-blue-700 disabled:opacity-50">
                  <Save className="w-4 h-4" /> {createGuard.isPending ? 'Saving…' : 'Save'}
                </button>
                <button onClick={() => setShowAddGuard(false)}
                  className="px-3 py-2 rounded-lg text-sm text-gray-600 hover:bg-gray-100">
                  Cancel
                </button>
              </div>
            </div>
          )}

          <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
            {guardsLoading ? (
              <div className="p-6"><TableSkeleton rows={5} /></div>
            ) : !guards?.length ? (
              <div className="p-8 text-center text-gray-400">No guards registered.</div>
            ) : (
              <table className="w-full text-sm">
                <thead className="bg-gray-50 border-b border-gray-100">
                  <tr>
                    {['Name', 'Employee ID', 'Role', 'Status', 'Actions'].map(h => (
                      <th key={h} className="text-left px-4 py-3 font-medium text-gray-600">{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50">
                  {guards.map(g => (
                    <tr key={g.id} className="hover:bg-gray-50">
                      <td className="px-4 py-3 font-medium text-gray-900">{g.name}</td>
                      <td className="px-4 py-3 text-gray-600">{g.employee_id ?? '—'}</td>
                      <td className="px-4 py-3 text-gray-600 capitalize">{g.role}</td>
                      <td className="px-4 py-3">
                        <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${g.status === 'active' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>
                          {g.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        {g.status === 'active' ? (
                          <button onClick={() => updateGuardStatus.mutate({ id: g.id, status: 'inactive' })}
                            className="text-xs text-red-600 hover:text-red-800">Deactivate</button>
                        ) : (
                          <button onClick={() => updateGuardStatus.mutate({ id: g.id, status: 'active' })}
                            className="text-xs text-green-600 hover:text-green-800">Activate</button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      )}

      {/* Sites tab */}
      {activeTab === 'sites' && (
        <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
          {sitesLoading ? (
            <div className="p-6"><TableSkeleton rows={3} /></div>
          ) : !sites?.length ? (
            <div className="p-8 text-center text-gray-400">No sites configured.</div>
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b border-gray-100">
                <tr>
                  {['Name', 'Address', 'Timezone', 'Keycloak Realm'].map(h => (
                    <th key={h} className="text-left px-4 py-3 font-medium text-gray-600">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-50">
                {sites.map(s => (
                  <tr key={s.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 font-medium text-gray-900">{s.name}</td>
                    <td className="px-4 py-3 text-gray-600">{s.address ?? '—'}</td>
                    <td className="px-4 py-3 text-gray-600">{s.timezone}</td>
                    <td className="px-4 py-3 text-gray-500 text-xs font-mono">{s.keycloak_realm ?? '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  )
}
