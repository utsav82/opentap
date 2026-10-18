import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { api } from '../lib/api';
import { Card, PageHeader, LoadingSpinner, EmptyState, Badge, ErrorMessage } from '../components/Card';

function UserDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [user, setUser] = useState(null);
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    async function load() {
      try {
        const [u, evts] = await Promise.all([
          api.getUser(id),
          api.getUserEvents(id, { limit: '20' }),
        ]);
        setUser(u);
        setEvents(evts?.data || []);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    }
    load();
  }, [id]);

  if (loading) return <LoadingSpinner />;
  if (!user) return <ErrorMessage error={error} />;

  return (
    <div>
      <button onClick={() => navigate('/users')} className="text-sm text-indigo-600 hover:underline mb-4 inline-block">
        &larr; Back to Users
      </button>
      <PageHeader title={user.user_id} description={`Internal ID: ${user.id}`} />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
        <Card className="p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">Profile</h3>
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between"><dt className="text-gray-500">First Seen</dt><dd>{new Date(user.first_seen).toLocaleDateString()}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Last Seen</dt><dd>{new Date(user.last_seen).toLocaleDateString()}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Event Count</dt><dd className="font-semibold">{user.event_count}</dd></div>
          </dl>
        </Card>
        <Card className="p-5 lg:col-span-2">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">Properties</h3>
          {Object.keys(user.properties || {}).length > 0 ? (
            <pre className="text-xs bg-gray-50 p-3 rounded-lg overflow-auto max-h-48">
              {JSON.stringify(user.properties, null, 2)}
            </pre>
          ) : (
            <p className="text-gray-400 text-sm">No custom properties</p>
          )}
        </Card>
      </div>

      {/* Event timeline */}
      <Card>
        <div className="p-4 border-b border-gray-200">
          <h3 className="text-sm font-semibold text-gray-700">Recent Events</h3>
        </div>
        <div className="divide-y divide-gray-100">
          {events.map((evt) => (
            <div key={evt.id} className="p-4 flex items-center justify-between hover:bg-gray-50 cursor-pointer" onClick={() => navigate(`/events/${evt.id}`)}>
              <div className="flex items-center gap-3">
                <div className="w-2 h-2 rounded-full bg-indigo-500" />
                <div>
                  <Badge variant="info">{evt.event_name}</Badge>
                  <p className="text-xs text-gray-400 mt-0.5">
                    {Object.keys(evt.properties || {}).length} properties
                  </p>
                </div>
              </div>
              <span className="text-xs text-gray-400">{new Date(evt.timestamp).toLocaleString()}</span>
            </div>
          ))}
          {events.length === 0 && <EmptyState message="No events yet" />}
        </div>
      </Card>
    </div>
  );
}

function UserList() {
  const navigate = useNavigate();
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [cursor, setCursor] = useState('');
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState(null);
  const [page, setPage] = useState('');

  useEffect(() => {
    api.listUsers(page ? { limit: '50', cursor: page } : { limit: '50' })
      .then((res) => {
        setUsers((prev) => (page ? [...prev, ...(res.data || [])] : res.data || []));
        setCursor(res.next_cursor || '');
        setHasMore(res.has_more || false);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [page]);

  const loadMore = () => {
    setLoading(true);
    setError(null);
    setPage(cursor);
  };

  return (
    <div>
      <PageHeader title="Users" description="All tracked user profiles" />
      <ErrorMessage error={error} />
      <Card>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200">
                <th className="text-left p-4 text-gray-500 font-medium">User ID</th>
                <th className="text-left p-4 text-gray-500 font-medium">Events</th>
                <th className="text-left p-4 text-gray-500 font-medium">First Seen</th>
                <th className="text-left p-4 text-gray-500 font-medium">Last Seen</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr
                  key={u.id}
                  onClick={() => navigate(`/users/${u.id}`)}
                  className="border-b border-gray-100 hover:bg-gray-50 cursor-pointer transition-colors"
                >
                  <td className="p-4 font-mono text-sm font-medium text-gray-900">{u.user_id}</td>
                  <td className="p-4 text-gray-600">{u.event_count}</td>
                  <td className="p-4 text-gray-500 text-xs">{new Date(u.first_seen).toLocaleDateString()}</td>
                  <td className="p-4 text-gray-500 text-xs">{new Date(u.last_seen).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {loading && <LoadingSpinner />}
        {!loading && !error && users.length === 0 && <EmptyState message="No users yet" />}

        {hasMore && !loading && (
          <div className="p-4 text-center border-t border-gray-100">
            <button onClick={loadMore} className="text-sm text-indigo-600 hover:underline font-medium">
              Load more
            </button>
          </div>
        )}
      </Card>
    </div>
  );
}

export { UserList, UserDetail };
