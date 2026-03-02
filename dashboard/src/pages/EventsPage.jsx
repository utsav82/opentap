import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { api } from '../lib/api';
import { Card, PageHeader, LoadingSpinner, EmptyState, Badge } from '../components/Card';

function EventDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [event, setEvent] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.getEvent(id).then(setEvent).catch(() => {}).finally(() => setLoading(false));
  }, [id]);

  if (loading) return <LoadingSpinner />;
  if (!event) return <EmptyState message="Event not found" />;

  return (
    <div>
      <button onClick={() => navigate('/events')} className="text-sm text-indigo-600 hover:underline mb-4 inline-block">
        &larr; Back to Events
      </button>
      <PageHeader title={event.event_name} description={`Event ID: ${event.id}`} />
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card className="p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">Details</h3>
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between"><dt className="text-gray-500">User ID</dt><dd className="font-mono">{event.user_id}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Timestamp</dt><dd>{new Date(event.timestamp).toLocaleString()}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Received At</dt><dd>{new Date(event.received_at).toLocaleString()}</dd></div>
          </dl>
        </Card>
        <Card className="p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">Properties</h3>
          {Object.keys(event.properties || {}).length > 0 ? (
            <pre className="text-xs bg-gray-50 p-3 rounded-lg overflow-auto max-h-64">
              {JSON.stringify(event.properties, null, 2)}
            </pre>
          ) : (
            <p className="text-gray-400 text-sm">No properties</p>
          )}
        </Card>
      </div>
    </div>
  );
}

function EventList() {
  const navigate = useNavigate();
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [filters, setFilters] = useState({ event_name: '', user_id: '' });
  const [cursor, setCursor] = useState('');
  const [hasMore, setHasMore] = useState(false);

  const fetchEvents = async (params = {}) => {
    setLoading(true);
    try {
      const query = { limit: '50', ...params };
      if (filters.event_name) query.event_name = filters.event_name;
      if (filters.user_id) query.user_id = filters.user_id;
      const res = await api.listEvents(query);
      setEvents(params.cursor ? (prev) => [...prev, ...(res.data || [])] : res.data || []);
      setCursor(res.next_cursor || '');
      setHasMore(res.has_more || false);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchEvents(); }, []);

  const handleFilter = (e) => {
    e.preventDefault();
    setEvents([]);
    fetchEvents();
  };

  return (
    <div>
      <PageHeader title="Events" description="All tracked events" />

      {/* Filters */}
      <Card className="p-4 mb-6">
        <form onSubmit={handleFilter} className="flex gap-3 items-end flex-wrap">
          <div>
            <label className="block text-xs text-gray-500 mb-1">Event Name</label>
            <input
              type="text"
              value={filters.event_name}
              onChange={(e) => setFilters({ ...filters, event_name: e.target.value })}
              placeholder="e.g. page_viewed"
              className="px-3 py-2 border border-gray-300 rounded-lg text-sm w-48 focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">User ID</label>
            <input
              type="text"
              value={filters.user_id}
              onChange={(e) => setFilters({ ...filters, user_id: e.target.value })}
              placeholder="e.g. user_123"
              className="px-3 py-2 border border-gray-300 rounded-lg text-sm w-48 focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none"
            />
          </div>
          <button
            type="submit"
            className="px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700 transition-colors"
          >
            Filter
          </button>
        </form>
      </Card>

      {/* Events Table */}
      <Card>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200">
                <th className="text-left p-4 text-gray-500 font-medium">Event</th>
                <th className="text-left p-4 text-gray-500 font-medium">User</th>
                <th className="text-left p-4 text-gray-500 font-medium">Properties</th>
                <th className="text-left p-4 text-gray-500 font-medium">Time</th>
              </tr>
            </thead>
            <tbody>
              {events.map((evt) => (
                <tr
                  key={evt.id}
                  onClick={() => navigate(`/events/${evt.id}`)}
                  className="border-b border-gray-100 hover:bg-gray-50 cursor-pointer transition-colors"
                >
                  <td className="p-4">
                    <Badge variant="info">{evt.event_name}</Badge>
                  </td>
                  <td className="p-4 font-mono text-xs text-gray-600">{evt.user_id}</td>
                  <td className="p-4 text-gray-500 text-xs">
                    {Object.keys(evt.properties || {}).length} props
                  </td>
                  <td className="p-4 text-gray-500 text-xs">
                    {new Date(evt.timestamp).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {loading && <LoadingSpinner />}
        {!loading && events.length === 0 && <EmptyState message="No events found" />}

        {hasMore && !loading && (
          <div className="p-4 text-center border-t border-gray-100">
            <button
              onClick={() => fetchEvents({ cursor })}
              className="text-sm text-indigo-600 hover:underline font-medium"
            >
              Load more
            </button>
          </div>
        )}
      </Card>
    </div>
  );
}

export { EventList, EventDetail };
