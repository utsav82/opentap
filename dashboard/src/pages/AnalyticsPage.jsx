import { useState } from 'react';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { api } from '../lib/api';
import { Card, PageHeader, ErrorMessage, EmptyState } from '../components/Card';

const field = 'px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 outline-none';
const button = 'px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700';

// Sequential single-hue ramp (light = low retention, dark = high).
const RAMP = ['#cde2fb', '#b7d3f6', '#9ec5f4', '#86b6ef', '#6da7ec', '#5598e7', '#3987e5', '#2a78d6', '#256abf', '#1c5cab', '#184f95', '#104281', '#0d366b'];
const PERIOD_MS = { day: 86400000, week: 604800000 };

const percent = (part, whole) => (whole ? `${Math.round((part / whole) * 100)}%` : '—');

function Funnel() {
  const [steps, setSteps] = useState('');
  const [timeWindow, setTimeWindow] = useState('30d');
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  const run = async (e) => {
    e.preventDefault();
    setError(null);
    try {
      const res = await api.funnel({ steps, time_window: timeWindow });
      setData(res.data);
    } catch (err) {
      setError(err.message);
    }
  };

  const first = data?.[0]?.users || 0;

  return (
    <Card className="p-5">
      <h3 className="text-sm font-semibold text-gray-700 mb-3">Funnel</h3>
      <form onSubmit={run} className="flex gap-2 flex-wrap mb-4">
        <input value={steps} onChange={(e) => setSteps(e.target.value)} required
          placeholder="signup, kyc_completed, first_trade" className={`flex-1 min-w-64 ${field}`} />
        <select value={timeWindow} onChange={(e) => setTimeWindow(e.target.value)} className={field}>
          <option value="7d">7 days</option><option value="30d">30 days</option><option value="90d">90 days</option><option value="all">All time</option>
        </select>
        <button type="submit" className={button}>Run</button>
      </form>
      <ErrorMessage error={error} />

      {data && (
        <>
          <ResponsiveContainer width="100%" height={Math.max(160, data.length * 48)}>
            <BarChart data={data} layout="vertical" margin={{ left: 8, right: 16 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" horizontal={false} />
              <XAxis type="number" tick={{ fontSize: 11 }} stroke="#9ca3af" allowDecimals={false} />
              <YAxis type="category" dataKey="event_name" tick={{ fontSize: 11 }} stroke="#9ca3af" width={130} />
              <Tooltip formatter={(users) => [`${users.toLocaleString()} users (${percent(users, first)} of first step)`, 'Reached']} />
              <Bar dataKey="users" fill="#6366f1" radius={[0, 4, 4, 0]} barSize={24} />
            </BarChart>
          </ResponsiveContainer>

          <table className="w-full text-sm mt-4">
            <thead>
              <tr className="border-b border-gray-200 text-gray-500">
                <th className="text-left py-2 font-medium">Step</th>
                <th className="text-right py-2 font-medium">Users</th>
                <th className="text-right py-2 font-medium">From previous</th>
                <th className="text-right py-2 font-medium">Overall</th>
              </tr>
            </thead>
            <tbody>
              {data.map((step, i) => (
                <tr key={i} className="border-b border-gray-100">
                  <td className="py-2 font-mono text-xs">{i + 1}. {step.event_name}</td>
                  <td className="py-2 text-right">{step.users.toLocaleString()}</td>
                  <td className="py-2 text-right text-gray-600">{i === 0 ? '—' : percent(step.users, data[i - 1].users)}</td>
                  <td className="py-2 text-right text-gray-600">{percent(step.users, first)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </Card>
  );
}

function Retention() {
  const [form, setForm] = useState({ start_event: '', return_event: '', interval: 'week', periods: '8' });
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);

  const set = (key) => (e) => setForm({ ...form, [key]: e.target.value });

  const run = async (e) => {
    e.preventDefault();
    setError(null);
    try {
      const res = await api.retention(form);
      setResult({ cohorts: res.data, interval: form.interval });
    } catch (err) {
      setError(err.message);
    }
  };

  const cellStyle = (rate) => {
    const step = Math.min(RAMP.length - 1, Math.round(rate * (RAMP.length - 1)));
    return { background: RAMP[step], color: step >= 6 ? '#ffffff' : '#1f2937' };
  };

  const periods = result?.cohorts[0]?.retained.length || 0;
  const unit = result?.interval === 'day' ? 'Day' : 'Week';

  return (
    <Card className="p-5">
      <h3 className="text-sm font-semibold text-gray-700 mb-3">Retention</h3>
      <form onSubmit={run} className="flex gap-2 flex-wrap mb-4">
        <input value={form.start_event} onChange={set('start_event')} required placeholder="Start event (e.g. signup)" className={`flex-1 min-w-48 ${field}`} />
        <input value={form.return_event} onChange={set('return_event')} placeholder="Return event (defaults to start)" className={`flex-1 min-w-48 ${field}`} />
        <select value={form.interval} onChange={set('interval')} className={field}>
          <option value="day">Daily</option><option value="week">Weekly</option>
        </select>
        <input type="number" min="1" max="52" value={form.periods} onChange={set('periods')} className={`w-20 ${field}`} />
        <button type="submit" className={button}>Run</button>
      </form>
      <ErrorMessage error={error} />

      {result && result.cohorts.length === 0 && <EmptyState message="No users did the start event in this range" />}
      {result && result.cohorts.length > 0 && (
        <div className="overflow-x-auto">
          <table className="text-xs border-separate" style={{ borderSpacing: 2 }}>
            <thead>
              <tr className="text-gray-500">
                <th className="text-left font-medium px-2 py-1">Cohort</th>
                <th className="text-right font-medium px-2 py-1">Users</th>
                {Array.from({ length: periods }, (_, p) => <th key={p} className="font-medium px-2 py-1">{unit} {p}</th>)}
              </tr>
            </thead>
            <tbody>
              {result.cohorts.map((c) => {
                const start = new Date(c.cohort);
                return (
                  <tr key={c.cohort}>
                    <td className="px-2 py-1.5 text-gray-700 whitespace-nowrap">{start.toLocaleDateString()}</td>
                    <td className="px-2 py-1.5 text-right text-gray-700">{c.users.toLocaleString()}</td>
                    {c.retained.map((n, p) => {
                      if (start.getTime() + p * PERIOD_MS[result.interval] > Date.now()) return <td key={p} />;
                      return (
                        <td key={p} className="px-2 py-1.5 text-center rounded min-w-12" style={cellStyle(c.users ? n / c.users : 0)}
                          title={`${start.toLocaleDateString()} cohort, ${unit.toLowerCase()} ${p}: ${n.toLocaleString()} of ${c.users.toLocaleString()} users`}>
                          {percent(n, c.users)}
                        </td>
                      );
                    })}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

export default function AnalyticsPage() {
  return (
    <div>
      <PageHeader title="Analytics" description="Funnels and retention cohorts" />
      <div className="space-y-6">
        <Funnel />
        <Retention />
      </div>
    </div>
  );
}
