import { useEffect, useState } from 'react';
import {
  AreaChart, Area, BarChart, Bar,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts';
import { Zap, Users, TrendingUp, Activity } from 'lucide-react';
import { api } from '../lib/api';
import { Card, StatCard, PageHeader, LoadingSpinner } from '../components/Card';

export default function DashboardPage() {
  const [dauData, setDauData] = useState([]);
  const [topEvents, setTopEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    async function load() {
      try {
        const [dau, top] = await Promise.all([
          api.activeUsers({ time_window: '30d', interval: 'day' }),
          api.topEvents({ time_window: '7d', limit: '10' }),
        ]);
        setDauData(
          (dau?.data || []).map((d) => ({
            date: new Date(d.timestamp).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
            users: d.count,
          }))
        );
        setTopEvents(dau?.data || []);
        setTopEvents(
          (top?.data || []).map((d) => ({
            name: d.event_name,
            count: d.count,
          }))
        );
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  const totalDAU = dauData.length > 0 ? dauData[dauData.length - 1]?.users : 0;
  const totalEvents = topEvents.reduce((sum, e) => sum + e.count, 0);

  if (loading) return <LoadingSpinner />;

  return (
    <div>
      <PageHeader title="Dashboard" description="Overview of your analytics" />

      {error && (
        <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
          {error}
        </div>
      )}

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <StatCard label="Daily Active Users" value={totalDAU.toLocaleString()} icon={Users} color="indigo" />
        <StatCard label="Events (7d)" value={totalEvents.toLocaleString()} icon={Zap} color="green" />
        <StatCard label="Event Types" value={topEvents.length} icon={Activity} color="blue" />
        <StatCard label="Trend" value={dauData.length > 1 ? '↗' : '—'} icon={TrendingUp} color="amber" />
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card className="p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-4">Daily Active Users (30d)</h3>
          {dauData.length > 0 ? (
            <ResponsiveContainer width="100%" height={260}>
              <AreaChart data={dauData}>
                <defs>
                  <linearGradient id="dauGradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#6366f1" stopOpacity={0.15} />
                    <stop offset="95%" stopColor="#6366f1" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                <XAxis dataKey="date" tick={{ fontSize: 11 }} stroke="#9ca3af" />
                <YAxis tick={{ fontSize: 11 }} stroke="#9ca3af" />
                <Tooltip />
                <Area type="monotone" dataKey="users" stroke="#6366f1" fill="url(#dauGradient)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          ) : (
            <p className="text-gray-400 text-center py-12">No data yet</p>
          )}
        </Card>

        <Card className="p-5">
          <h3 className="text-sm font-semibold text-gray-700 mb-4">Top Events (7d)</h3>
          {topEvents.length > 0 ? (
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={topEvents} layout="vertical">
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                <XAxis type="number" tick={{ fontSize: 11 }} stroke="#9ca3af" />
                <YAxis type="category" dataKey="name" tick={{ fontSize: 11 }} stroke="#9ca3af" width={120} />
                <Tooltip />
                <Bar dataKey="count" fill="#6366f1" radius={[0, 4, 4, 0]} />
              </BarChart>
            </ResponsiveContainer>
          ) : (
            <p className="text-gray-400 text-center py-12">No data yet</p>
          )}
        </Card>
      </div>
    </div>
  );
}
