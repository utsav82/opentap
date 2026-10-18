import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import DashboardPage from './pages/DashboardPage';
import { EventList, EventDetail } from './pages/EventsPage';
import { UserList, UserDetail } from './pages/UsersPage';
import { SegmentList, SegmentDetail } from './pages/SegmentsPage';
import { SchemaList, SchemaDetail } from './pages/SchemasPage';
import SettingsPage from './pages/SettingsPage';
import AnalyticsPage from './pages/AnalyticsPage';

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/events" element={<EventList />} />
          <Route path="/events/:id" element={<EventDetail />} />
          <Route path="/users" element={<UserList />} />
          <Route path="/users/:id" element={<UserDetail />} />
          <Route path="/analytics" element={<AnalyticsPage />} />
          <Route path="/segments" element={<SegmentList />} />
          <Route path="/segments/:id" element={<SegmentDetail />} />
          <Route path="/schemas" element={<SchemaList />} />
          <Route path="/schemas/:eventName" element={<SchemaDetail />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
