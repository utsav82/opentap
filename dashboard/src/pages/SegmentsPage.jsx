import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { api } from '../lib/api';
import { Card, PageHeader, LoadingSpinner, EmptyState, Badge } from '../components/Card';
import { Plus, Trash2, Users } from 'lucide-react';

// ---- Segment Builder Modal ----
function SegmentBuilder({ onSave, onCancel, initial }) {
  const [name, setName] = useState(initial?.name || '');
  const [description, setDescription] = useState(initial?.description || '');
  const [operator, setOperator] = useState(initial?.conditions?.operator || 'AND');
  const [rules, setRules] = useState(initial?.conditions?.rules || [
    { type: 'event', event_name: '', time_window: '30d', count_operator: 'gte', count_value: 1, property_filters: [] },
  ]);

  const addRule = () => {
    setRules([...rules, { type: 'event', event_name: '', time_window: '30d', count_operator: 'gte', count_value: 1, property_filters: [] }]);
  };

  const removeRule = (idx) => {
    setRules(rules.filter((_, i) => i !== idx));
  };

  const updateRule = (idx, field, value) => {
    const updated = [...rules];
    updated[idx] = { ...updated[idx], [field]: value };
    setRules(updated);
  };

  const handleSubmit = (e) => {
    e.preventDefault();
    onSave({
      name,
      description,
      conditions: { operator, rules },
    });
  };

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
      <Card className="w-full max-w-2xl max-h-[90vh] overflow-y-auto">
        <form onSubmit={handleSubmit}>
          <div className="p-5 border-b border-gray-200">
            <h2 className="text-lg font-semibold text-gray-900">{initial ? 'Edit Segment' : 'Create Segment'}</h2>
          </div>

          <div className="p-5 space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
              <input value={name} onChange={(e) => setName(e.target.value)} required
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 outline-none" placeholder="High Value Users" />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
              <input value={description} onChange={(e) => setDescription(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 outline-none" placeholder="Optional description" />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Match</label>
              <select value={operator} onChange={(e) => setOperator(e.target.value)}
                className="px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 outline-none">
                <option value="AND">ALL rules (AND)</option>
                <option value="OR">ANY rule (OR)</option>
              </select>
            </div>

            <div className="space-y-3">
              <label className="block text-sm font-medium text-gray-700">Rules</label>
              {rules.map((rule, idx) => (
                <div key={idx} className="p-3 border border-gray-200 rounded-lg bg-gray-50 space-y-2">
                  <div className="flex items-center gap-2">
                    <select value={rule.type} onChange={(e) => updateRule(idx, 'type', e.target.value)}
                      className="px-2 py-1.5 border border-gray-300 rounded text-xs focus:ring-2 focus:ring-indigo-500 outline-none">
                      <option value="event">Event</option>
                      <option value="user_property">User Property</option>
                    </select>

                    {rule.type === 'event' && (
                      <>
                        <input value={rule.event_name || ''} onChange={(e) => updateRule(idx, 'event_name', e.target.value)}
                          placeholder="Event name" className="flex-1 px-2 py-1.5 border border-gray-300 rounded text-xs focus:ring-2 focus:ring-indigo-500 outline-none" />
                        <select value={rule.time_window || '30d'} onChange={(e) => updateRule(idx, 'time_window', e.target.value)}
                          className="px-2 py-1.5 border border-gray-300 rounded text-xs">
                          <option value="1h">1h</option><option value="24h">24h</option><option value="7d">7d</option>
                          <option value="30d">30d</option><option value="90d">90d</option><option value="all">All</option>
                        </select>
                        <select value={rule.count_operator || 'gte'} onChange={(e) => updateRule(idx, 'count_operator', e.target.value)}
                          className="px-2 py-1.5 border border-gray-300 rounded text-xs">
                          <option value="gte">&ge;</option><option value="gt">&gt;</option><option value="eq">=</option>
                          <option value="lt">&lt;</option><option value="lte">&le;</option>
                        </select>
                        <input type="number" value={rule.count_value ?? 1} onChange={(e) => updateRule(idx, 'count_value', parseInt(e.target.value) || 0)}
                          className="w-16 px-2 py-1.5 border border-gray-300 rounded text-xs focus:ring-2 focus:ring-indigo-500 outline-none" />
                      </>
                    )}

                    {rule.type === 'user_property' && (
                      <>
                        <input value={rule.property || ''} onChange={(e) => updateRule(idx, 'property', e.target.value)}
                          placeholder="Property" className="flex-1 px-2 py-1.5 border border-gray-300 rounded text-xs focus:ring-2 focus:ring-indigo-500 outline-none" />
                        <select value={rule.operator || 'eq'} onChange={(e) => updateRule(idx, 'operator', e.target.value)}
                          className="px-2 py-1.5 border border-gray-300 rounded text-xs">
                          <option value="eq">=</option><option value="neq">!=</option><option value="gt">&gt;</option>
                          <option value="gte">&ge;</option><option value="lt">&lt;</option><option value="lte">&le;</option>
                          <option value="contains">contains</option><option value="is_set">is set</option><option value="is_not_set">is not set</option>
                        </select>
                        {!['is_set', 'is_not_set'].includes(rule.operator) && (
                          <input value={rule.value ?? ''} onChange={(e) => updateRule(idx, 'value', e.target.value)}
                            placeholder="Value" className="w-28 px-2 py-1.5 border border-gray-300 rounded text-xs focus:ring-2 focus:ring-indigo-500 outline-none" />
                        )}
                      </>
                    )}

                    <button type="button" onClick={() => removeRule(idx)} className="text-red-400 hover:text-red-600 p-1">
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>
              ))}
              <button type="button" onClick={addRule} className="flex items-center gap-1 text-xs text-indigo-600 hover:underline font-medium">
                <Plus size={14} /> Add Rule
              </button>
            </div>
          </div>

          <div className="p-5 border-t border-gray-200 flex justify-end gap-3">
            <button type="button" onClick={onCancel} className="px-4 py-2 text-sm text-gray-600 hover:text-gray-900">Cancel</button>
            <button type="submit" className="px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700">
              {initial ? 'Update' : 'Create'}
            </button>
          </div>
        </form>
      </Card>
    </div>
  );
}

// ---- Segment Detail ----
function SegmentDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [segment, setSegment] = useState(null);
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const [seg, u] = await Promise.all([
          api.getSegment(id),
          api.getSegmentUsers(id, { limit: '20' }),
        ]);
        setSegment(seg);
        setUsers(u?.data || []);
      } catch (err) {
        console.error(err);
      } finally {
        setLoading(false);
      }
    }
    load();
  }, [id]);

  if (loading) return <LoadingSpinner />;
  if (!segment) return <EmptyState message="Segment not found" />;

  return (
    <div>
      <button onClick={() => navigate('/segments')} className="text-sm text-indigo-600 hover:underline mb-4 inline-block">
        &larr; Back to Segments
      </button>
      <PageHeader title={segment.name} description={segment.description || `${segment.user_count ?? 0} users`} />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
        <Card className="p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">Info</h3>
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between"><dt className="text-gray-500">Users</dt><dd className="font-semibold">{segment.user_count ?? 0}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Created</dt><dd>{new Date(segment.created_at).toLocaleDateString()}</dd></div>
          </dl>
        </Card>
        <Card className="p-5 lg:col-span-2">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">Conditions</h3>
          <pre className="text-xs bg-gray-50 p-3 rounded-lg overflow-auto max-h-48">
            {JSON.stringify(segment.conditions, null, 2)}
          </pre>
        </Card>
      </div>

      <Card>
        <div className="p-4 border-b border-gray-200">
          <h3 className="text-sm font-semibold text-gray-700 flex items-center gap-2"><Users size={16} /> Users in Segment</h3>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200">
                <th className="text-left p-4 text-gray-500 font-medium">User ID</th>
                <th className="text-left p-4 text-gray-500 font-medium">Events</th>
                <th className="text-left p-4 text-gray-500 font-medium">Last Seen</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id} onClick={() => navigate(`/users/${u.id}`)} className="border-b border-gray-100 hover:bg-gray-50 cursor-pointer">
                  <td className="p-4 font-mono text-sm">{u.user_id}</td>
                  <td className="p-4 text-gray-600">{u.event_count}</td>
                  <td className="p-4 text-gray-500 text-xs">{new Date(u.last_seen).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {users.length === 0 && <EmptyState message="No users matched yet" />}
        </div>
      </Card>
    </div>
  );
}

// ---- Segment List ----
function SegmentList() {
  const navigate = useNavigate();
  const [segments, setSegments] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showBuilder, setShowBuilder] = useState(false);

  const fetchSegments = async () => {
    setLoading(true);
    try {
      const res = await api.listSegments({ limit: '50' });
      setSegments(res.data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchSegments(); }, []);

  const handleCreate = async (data) => {
    try {
      await api.createSegment(data);
      setShowBuilder(false);
      fetchSegments();
    } catch (err) {
      alert(err.message);
    }
  };

  const handleDelete = async (e, id) => {
    e.stopPropagation();
    if (!confirm('Delete this segment?')) return;
    try {
      await api.deleteSegment(id);
      fetchSegments();
    } catch (err) {
      alert(err.message);
    }
  };

  return (
    <div>
      <PageHeader
        title="Segments"
        description="Define audience segments"
        action={
          <button onClick={() => setShowBuilder(true)} className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700">
            <Plus size={16} /> Create Segment
          </button>
        }
      />

      {showBuilder && <SegmentBuilder onSave={handleCreate} onCancel={() => setShowBuilder(false)} />}

      <Card>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200">
                <th className="text-left p-4 text-gray-500 font-medium">Name</th>
                <th className="text-left p-4 text-gray-500 font-medium">Description</th>
                <th className="text-left p-4 text-gray-500 font-medium">Created</th>
                <th className="text-left p-4 text-gray-500 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {segments.map((seg) => (
                <tr key={seg.id} onClick={() => navigate(`/segments/${seg.id}`)} className="border-b border-gray-100 hover:bg-gray-50 cursor-pointer transition-colors">
                  <td className="p-4 font-medium text-gray-900">{seg.name}</td>
                  <td className="p-4 text-gray-500 text-xs truncate max-w-xs">{seg.description || '—'}</td>
                  <td className="p-4 text-gray-500 text-xs">{new Date(seg.created_at).toLocaleDateString()}</td>
                  <td className="p-4">
                    <button onClick={(e) => handleDelete(e, seg.id)} className="text-red-400 hover:text-red-600">
                      <Trash2 size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {loading && <LoadingSpinner />}
        {!loading && segments.length === 0 && <EmptyState message="No segments created yet" />}
      </Card>
    </div>
  );
}

export { SegmentList, SegmentDetail };
