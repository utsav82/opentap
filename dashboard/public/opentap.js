/*
 * OpenTap browser SDK. Use an ingest-only API key: it is visible to anyone who loads the page.
 *
 *   <script src="https://analytics.example.com/opentap.js"></script>
 *   <script>
 *     OpenTap.init({ apiKey: 'ot_...', host: 'https://analytics.example.com' });
 *     OpenTap.track('page_viewed', { page: location.pathname });
 *     OpenTap.identify('user_123', { plan: 'pro' });
 *   </script>
 */
(function (global) {
  const STORAGE_KEY = 'opentap_user';
  const MAX_BATCH = 100;
  const FLUSH_INTERVAL = 2000;

  let config = null;
  let queue = [];
  let state = {};

  function save() {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(state)); } catch { /* storage unavailable */ }
  }

  function restore() {
    try { return JSON.parse(localStorage.getItem(STORAGE_KEY)) || {}; } catch { return {}; }
  }

  function send(path, body) {
    return fetch(config.host + path, {
      method: 'POST',
      keepalive: true,
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${config.apiKey}` },
      body: JSON.stringify(body),
    }).then((res) => {
      if (!res.ok && res.status !== 400) throw new Error(`OpenTap request failed: ${res.status}`);
    });
  }

  function event(eventName, fields) {
    return { id: crypto.randomUUID(), user_id: state.userId, event_name: eventName, timestamp: new Date().toISOString(), ...fields };
  }

  function flush() {
    if (!config || queue.length === 0) return Promise.resolve();
    const batch = queue.splice(0, MAX_BATCH);
    // Failed batches go back on the queue; events carry their own id and timestamp, so resends are deduplicated.
    return send('/v1/events/batch', { events: batch }).catch(() => { queue = batch.concat(queue).slice(-1000); });
  }

  const OpenTap = {
    init(options) {
      config = { host: '', ...options };
      state = restore();
      if (!state.userId) {
        state.userId = state.anonymousId = crypto.randomUUID();
        save();
      }
      setInterval(flush, FLUSH_INTERVAL);
      addEventListener('pagehide', flush);
    },

    track(eventName, properties = {}) {
      queue.push(event(eventName, { properties }));
      if (queue.length >= MAX_BATCH) flush();
    },

    identify(userId, userProperties = {}) {
      const anonymousId = state.anonymousId;
      state = { userId };
      save();
      queue.push(event('identify', { user_properties: userProperties }));
      if (anonymousId && anonymousId !== userId) {
        return flush().then(() => send('/v1/identify', { anonymous_id: anonymousId, user_id: userId }));
      }
      return flush();
    },

    reset() {
      state = { userId: crypto.randomUUID() };
      state.anonymousId = state.userId;
      save();
    },

    flush,
  };

  global.OpenTap = OpenTap;
})(window);
