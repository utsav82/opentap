import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { api } from '../lib/api';
import { Card, PageHeader, LoadingSpinner, EmptyState, Badge, ErrorMessage } from '../components/Card';

function SchemaDetail() {
  const { eventName } = useParams();
  const navigate = useNavigate();
  const [schema, setSchema] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    api.getSchema(eventName).then(setSchema).catch((err) => setError(err.message)).finally(() => setLoading(false));
  }, [eventName]);

  if (loading) return <LoadingSpinner />;
  if (!schema) return <ErrorMessage error={error} />;

  return (
    <div>
      <button onClick={() => navigate('/schemas')} className="text-sm text-indigo-600 hover:underline mb-4 inline-block">
        &larr; Back to Schemas
      </button>
      <PageHeader title={schema.event_name} description={`${schema.properties?.length || 0} properties discovered`} />

      <Card>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200">
                <th className="text-left p-4 text-gray-500 font-medium">Property</th>
                <th className="text-left p-4 text-gray-500 font-medium">Type</th>
                <th className="text-left p-4 text-gray-500 font-medium">Distinct Values</th>
                <th className="text-left p-4 text-gray-500 font-medium">Input Type</th>
              </tr>
            </thead>
            <tbody>
              {(schema.properties || []).map((prop) => (
                <tr key={prop.name} className="border-b border-gray-100">
                  <td className="p-4 font-mono text-sm font-medium text-gray-900">{prop.name}</td>
                  <td className="p-4"><Badge variant={typeColor(prop.type)}>{prop.type}</Badge></td>
                  <td className="p-4 text-gray-600">{prop.distinct_values_count}</td>
                  <td className="p-4 text-gray-500 text-xs">{prop.distinct_values_count <= 20 ? 'Dropdown' : 'Text input'}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {(schema.properties || []).length === 0 && <EmptyState message="No properties discovered" />}
        </div>
      </Card>
    </div>
  );
}

function SchemaList() {
  const navigate = useNavigate();
  const [schemas, setSchemas] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    api.listSchemas()
      .then((res) => setSchemas(res?.data || []))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  return (
    <div>
      <PageHeader title="Schemas" description="Auto-discovered event types and properties" />
      <ErrorMessage error={error} />
      <Card>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200">
                <th className="text-left p-4 text-gray-500 font-medium">Event Name</th>
                <th className="text-left p-4 text-gray-500 font-medium">Properties</th>
                <th className="text-left p-4 text-gray-500 font-medium">First Seen</th>
                <th className="text-left p-4 text-gray-500 font-medium">Last Seen</th>
              </tr>
            </thead>
            <tbody>
              {schemas.map((s) => (
                <tr key={s.event_name} onClick={() => navigate(`/schemas/${encodeURIComponent(s.event_name)}`)} className="border-b border-gray-100 hover:bg-gray-50 cursor-pointer transition-colors">
                  <td className="p-4 font-mono text-sm font-medium text-gray-900">{s.event_name}</td>
                  <td className="p-4 text-gray-600">{s.property_count}</td>
                  <td className="p-4 text-gray-500 text-xs">{new Date(s.first_seen).toLocaleDateString()}</td>
                  <td className="p-4 text-gray-500 text-xs">{new Date(s.last_seen).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {loading && <LoadingSpinner />}
        {!loading && !error && schemas.length === 0 && <EmptyState message="No event schemas discovered yet. Start sending events!" />}
      </Card>
    </div>
  );
}

function typeColor(type) {
  switch (type) {
    case 'string': return 'info';
    case 'number': return 'success';
    case 'boolean': return 'warning';
    default: return 'default';
  }
}

export { SchemaList, SchemaDetail };
