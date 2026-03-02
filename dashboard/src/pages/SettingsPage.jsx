import { useEffect, useState } from 'react';
import { api, setApiKey, getApiKey } from '../lib/api';
import { Card, PageHeader, LoadingSpinner, EmptyState, Badge } from '../components/Card';
import { Plus, Trash2, Key, Copy, Check } from 'lucide-react';

export default function SettingsPage() {
  const [projects, setProjects] = useState([]);
  const [selectedProject, setSelectedProject] = useState(null);
  const [keys, setKeys] = useState([]);
  const [loading, setLoading] = useState(true);
  const [newProjectName, setNewProjectName] = useState('');
  const [newKeyLabel, setNewKeyLabel] = useState('');
  const [createdKey, setCreatedKey] = useState(null);
  const [copied, setCopied] = useState(false);
  const [currentApiKey, setCurrentApiKey] = useState(getApiKey());

  // Load projects
  useEffect(() => {
    api.listProjects()
      .then((res) => {
        const list = res?.data || [];
        setProjects(list);
        if (list.length > 0 && !selectedProject) {
          setSelectedProject(list[0]);
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  // Load keys for selected project
  // Note: no listKeys endpoint in spec, so we show created keys in memory
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
      const res = await api.createApiKey(selectedProject.id, newKeyLabel.trim() || 'default');
      setCreatedKey(res);
      setNewKeyLabel('');
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

  const handleApiKeyInput = (e) => {
    const key = e.target.value;
    setCurrentApiKey(key);
    setApiKey(key);
  };

  if (loading) return <LoadingSpinner />;

  return (
    <div>
      <PageHeader title="Settings" description="Manage projects and API keys" />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* API Key Configuration */}
        <Card className="p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-3 flex items-center gap-2"><Key size={16} /> Active API Key</h3>
          <p className="text-xs text-gray-500 mb-3">Set the API key used for all dashboard requests.</p>
          <input
            type="text"
            value={currentApiKey}
            onChange={handleApiKeyInput}
            placeholder="ot_..."
            className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm font-mono focus:ring-2 focus:ring-indigo-500 outline-none"
          />
          {currentApiKey && (
            <p className="text-xs text-green-600 mt-2">API key is set</p>
          )}
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
              className="flex-1 px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 outline-none"
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

      {/* Generate API Key */}
      {selectedProject && (
        <Card className="mt-6 p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">
            Generate API Key for <span className="text-indigo-600">{selectedProject.name}</span>
          </h3>
          <form onSubmit={handleCreateKey} className="flex gap-2 mb-4">
            <input
              type="text"
              value={newKeyLabel}
              onChange={(e) => setNewKeyLabel(e.target.value)}
              placeholder="Key label (e.g. Production)"
              className="flex-1 px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 outline-none"
            />
            <button type="submit" className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700">
              <Key size={14} /> Generate
            </button>
          </form>

          {createdKey && (
            <div className="p-4 bg-green-50 border border-green-200 rounded-lg">
              <p className="text-sm font-medium text-green-800 mb-2">API Key created! Copy it now — it won't be shown again.</p>
              <div className="flex items-center gap-2">
                <code className="flex-1 p-2 bg-white border border-green-300 rounded text-xs font-mono break-all">
                  {createdKey.key}
                </code>
                <button onClick={() => handleCopyKey(createdKey.key)} className="p-2 text-green-600 hover:text-green-800">
                  {copied ? <Check size={16} /> : <Copy size={16} />}
                </button>
                <button
                  onClick={() => handleSetActiveKey(createdKey.key)}
                  className="px-3 py-1.5 bg-green-600 text-white rounded text-xs font-medium hover:bg-green-700"
                >
                  Use this key
                </button>
              </div>
              <div className="mt-2 text-xs text-green-700">
                Prefix: {createdKey.key_prefix} &middot; Label: {createdKey.label}
              </div>
            </div>
          )}
        </Card>
      )}
    </div>
  );
}
