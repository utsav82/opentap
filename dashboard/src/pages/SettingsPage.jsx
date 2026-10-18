import { useEffect, useState } from 'react';
import { api, setApiKey, getApiKey, setAdminToken, getAdminToken } from '../lib/api';
import { Card, PageHeader, EmptyState, Badge, ErrorMessage } from '../components/Card';
import { Plus, Key, Copy, Check, Trash2 } from 'lucide-react';

const input = 'w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 outline-none';

export default function SettingsPage() {
  const [projects, setProjects] = useState([]);
  const [selectedProject, setSelectedProject] = useState(null);
  const [keys, setKeys] = useState([]);
  const [error, setError] = useState(null);
  const [newProjectName, setNewProjectName] = useState('');
  const [newKeyLabel, setNewKeyLabel] = useState('');
  const [newKeyScope, setNewKeyScope] = useState('full');
  const [projectsVersion, setProjectsVersion] = useState(0);
  const [keysVersion, setKeysVersion] = useState(0);
  const [createdKey, setCreatedKey] = useState(null);
  const [copied, setCopied] = useState(false);
  const [adminToken, setAdminTokenInput] = useState(getAdminToken());
  const [currentApiKey, setCurrentApiKey] = useState(getApiKey());

  useEffect(() => {
    api.listProjects()
      .then((res) => {
        const list = res?.data || [];
        setProjects(list);
        setSelectedProject((current) => current || list[0] || null);
        setError(null);
      })
      .catch((err) => setError(err.message));
  }, [projectsVersion]);

  useEffect(() => {
    if (!selectedProject) return;
    api.listApiKeys(selectedProject.id)
      .then((res) => setKeys(res?.data || []))
      .catch((err) => setError(err.message));
  }, [selectedProject, keysVersion]);

  const reloadKeys = () => setKeysVersion((v) => v + 1);

  const handleSaveCredentials = (e) => {
    e.preventDefault();
    setAdminToken(adminToken.trim());
    setApiKey(currentApiKey.trim());
    setProjectsVersion((v) => v + 1);
  };

  const handleCreateProject = async (e) => {
    e.preventDefault();
    if (!newProjectName.trim()) return;
    try {
      const project = await api.createProject(newProjectName.trim());
      setProjects([project, ...projects]);
      setSelectedProject(project);
      setNewProjectName('');
    } catch (err) {
      alert(err.message);
    }
  };

  const handleCreateKey = async (e) => {
    e.preventDefault();
    if (!selectedProject) return;
    try {
      const res = await api.createApiKey(selectedProject.id, newKeyLabel.trim() || 'default', newKeyScope);
      setCreatedKey(res);
      setNewKeyLabel('');
      reloadKeys();
    } catch (err) {
      alert(err.message);
    }
  };

  const handleRevokeKey = async (key) => {
    if (!confirm(`Revoke key ${key.key_prefix}…?`)) return;
    try {
      await api.revokeApiKey(selectedProject.id, key.id);
      reloadKeys();
    } catch (err) {
      alert(err.message);
    }
  };

  const handleCopyKey = async (key) => {
    await navigator.clipboard.writeText(key);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleSetActiveKey = (key) => {
    setApiKey(key);
    setCurrentApiKey(key);
  };

  return (
    <div>
      <PageHeader title="Settings" description="Manage projects and API keys" />
      <ErrorMessage error={error} />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Credentials */}
        <Card className="p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-3 flex items-center gap-2"><Key size={16} /> Credentials</h3>
          <form onSubmit={handleSaveCredentials} className="space-y-3">
            <div>
              <label className="block text-xs text-gray-500 mb-1">Admin token (manages projects and keys)</label>
              <input type="password" value={adminToken} onChange={(e) => setAdminTokenInput(e.target.value)} placeholder="ADMIN_TOKEN" className={`${input} font-mono`} />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Project API key (used for all analytics data)</label>
              <input type="text" value={currentApiKey} onChange={(e) => setCurrentApiKey(e.target.value)} placeholder="ot_..." className={`${input} font-mono`} />
            </div>
            <button type="submit" className="px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700">Save</button>
          </form>
        </Card>

        {/* Create Project */}
        <Card className="p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">Create Project</h3>
          <form onSubmit={handleCreateProject} className="flex gap-2">
            <input
              type="text"
              value={newProjectName}
              onChange={(e) => setNewProjectName(e.target.value)}
              placeholder="Project name"
              className={`flex-1 ${input}`}
            />
            <button type="submit" className="px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700">
              <Plus size={16} />
            </button>
          </form>
        </Card>
      </div>

      {/* Projects List */}
      <Card className="mt-6">
        <div className="p-4 border-b border-gray-200">
          <h3 className="text-sm font-semibold text-gray-700">Projects</h3>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200">
                <th className="text-left p-4 text-gray-500 font-medium">Name</th>
                <th className="text-left p-4 text-gray-500 font-medium">ID</th>
                <th className="text-left p-4 text-gray-500 font-medium">Created</th>
                <th className="text-left p-4 text-gray-500 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {projects.map((p) => (
                <tr key={p.id} className={`border-b border-gray-100 ${selectedProject?.id === p.id ? 'bg-indigo-50' : ''}`}>
                  <td className="p-4 font-medium text-gray-900">{p.name}</td>
                  <td className="p-4 font-mono text-xs text-gray-500">{p.id}</td>
                  <td className="p-4 text-gray-500 text-xs">{new Date(p.created_at).toLocaleDateString()}</td>
                  <td className="p-4">
                    <button
                      onClick={() => setSelectedProject(p)}
                      className={`text-xs font-medium ${selectedProject?.id === p.id ? 'text-indigo-600' : 'text-gray-400 hover:text-gray-600'}`}
                    >
                      {selectedProject?.id === p.id ? 'Selected' : 'Select'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {projects.length === 0 && <EmptyState message="No projects yet. Create one above." />}
        </div>
      </Card>

      {/* API Keys */}
      {selectedProject && (
        <Card className="mt-6 p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">
            API Keys for <span className="text-indigo-600">{selectedProject.name}</span>
          </h3>
          <form onSubmit={handleCreateKey} className="flex gap-2 mb-4">
            <input
              type="text"
              value={newKeyLabel}
              onChange={(e) => setNewKeyLabel(e.target.value)}
              placeholder="Key label (e.g. Production)"
              className={`flex-1 ${input}`}
            />
            <select value={newKeyScope} onChange={(e) => setNewKeyScope(e.target.value)}
              className="px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 outline-none">
              <option value="full">Full access</option>
              <option value="ingest">Ingest only (safe for apps)</option>
            </select>
            <button type="submit" className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700">
              <Key size={14} /> Generate
            </button>
          </form>

          {createdKey && (
            <div className="p-4 mb-4 bg-green-50 border border-green-200 rounded-lg">
              <p className="text-sm font-medium text-green-800 mb-2">API Key created! Copy it now — it won't be shown again.</p>
              <div className="flex items-center gap-2">
                <code className="flex-1 p-2 bg-white border border-green-300 rounded text-xs font-mono break-all">
                  {createdKey.key}
                </code>
                <button onClick={() => handleCopyKey(createdKey.key)} className="p-2 text-green-600 hover:text-green-800">
                  {copied ? <Check size={16} /> : <Copy size={16} />}
                </button>
                {createdKey.scope !== 'ingest' && (
                  <button
                    onClick={() => handleSetActiveKey(createdKey.key)}
                    className="px-3 py-1.5 bg-green-600 text-white rounded text-xs font-medium hover:bg-green-700"
                  >
                    Use this key
                  </button>
                )}
              </div>
            </div>
          )}

          <table className="w-full text-sm">
            <tbody>
              {keys.map((k) => (
                <tr key={k.id} className="border-b border-gray-100">
                  <td className="py-3 font-mono text-xs">{k.key_prefix}…</td>
                  <td className="py-3 text-gray-600">{k.label || '—'}</td>
                  <td className="py-3"><Badge variant={k.scope === 'ingest' ? 'info' : 'default'}>{k.scope === 'ingest' ? 'Ingest only' : 'Full access'}</Badge></td>
                  <td className="py-3 text-gray-500 text-xs">{new Date(k.created_at).toLocaleDateString()}</td>
                  <td className="py-3 text-right">
                    {k.revoked_at ? (
                      <Badge variant="danger">Revoked</Badge>
                    ) : (
                      <button onClick={() => handleRevokeKey(k)} className="text-red-400 hover:text-red-600" title="Revoke">
                        <Trash2 size={14} />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {keys.length === 0 && <EmptyState message="No keys for this project yet" />}
        </Card>
      )}
    </div>
  );
}
