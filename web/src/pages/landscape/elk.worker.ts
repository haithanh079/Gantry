import ELK from 'elkjs/lib/elk.bundled.js';

const elk = new ELK();

addEventListener('message', async (event: MessageEvent) => {
  try {
    const layout = await elk.layout(event.data);
    postMessage({ ok: true, layout });
  } catch (err) {
    postMessage({ ok: false, error: err instanceof Error ? err.message : 'layout failed' });
  }
});
