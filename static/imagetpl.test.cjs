const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

test('JPEG download submits the current editor text and styles', async () => {
    let factory;
    let submitted;
    let clicked = false;
    const button = { disabled: false };
    const form = {
        action: '/preview-image/export',
        querySelector: () => button,
        controls: new Map([['message', ''], ['description', '']]),
    };
    class FormData {
        constructor(source) { this.values = new Map(source.controls); }
        set(key, value) { this.values.set(key, value); }
        get(key) { return this.values.get(key); }
    }
    const context = {
        FormData,
        URL: { createObjectURL: () => 'blob:test', revokeObjectURL: () => {} },
        setTimeout: () => {},
        document: {
            body: { append: () => {} },
            createElement: () => ({ click: () => { clicked = true; }, remove: () => {} }),
        },
        window: {
            Alpine: {
                data: (_name, value) => { factory = value; },
                store: () => ({ csrf: 'test-token' }),
            },
            dispatchEvent: () => assert.fail('download reported an error'),
        },
        fetch: async (_url, options) => {
            submitted = options;
            return { ok: true, blob: async () => ({}), headers: { get: () => 'attachment; filename="preview.jpg"' } };
        },
    };
    vm.runInNewContext(readFileSync(join(__dirname, 'imagetpl.js'), 'utf8'), context);

    const editor = factory();
    editor.config = { fields: ['message', 'description'], remember: false };
    editor.fields = { message: 'Unavailable', description: 'First line\nSecond line' };
    editor.styles = {
        message: { font: 'bold', color: '#ff0000' },
        description: { font: 'regular', color: '#00aaff' },
    };
    await editor.download({ target: form });

    assert.equal(submitted.body.get('message'), 'Unavailable');
    assert.equal(submitted.body.get('description'), 'First line\nSecond line');
    assert.equal(submitted.body.get('_imagetpl_font_message'), 'bold');
    assert.equal(submitted.body.get('_imagetpl_color_description'), '#00aaff');
    assert.equal(submitted.headers['X-CSRF-Token'], 'test-token');
    assert.equal(button.disabled, false);
    assert.equal(clicked, true);
});

test('a locked page reports why instead of downloading', async () => {
    let factory;
    let fetched = false;
    const notices = [];
    const context = {
        window: {
            Alpine: { data: (_name, value) => { factory = value; }, store: () => ({ csrf: '' }) },
            dispatchEvent: (event) => notices.push(event.detail),
        },
        CustomEvent: class { constructor(_type, init) { this.detail = init.detail; } },
        fetch: async () => { fetched = true; },
    };
    vm.runInNewContext(readFileSync(join(__dirname, 'imagetpl.js'), 'utf8'), context);

    const editor = factory();
    editor.config = { fields: [], remember: false, exportLocked: 'Needs a licence.' };
    await editor.download({ target: { querySelector: () => ({}) } });

    assert.equal(fetched, false);
    assert.equal(notices.length, 1);
    assert.equal(notices[0].description, 'Needs a licence.');
});

test('a refused download shows the server message', async () => {
    let factory;
    const notices = [];
    const context = {
        FormData: class { set() {} },
        window: {
            Alpine: { data: (_name, value) => { factory = value; }, store: () => ({ csrf: '' }) },
            dispatchEvent: (event) => notices.push(event.detail),
        },
        CustomEvent: class { constructor(_type, init) { this.detail = init.detail; } },
        fetch: async () => ({ ok: false, status: 403, text: async () => 'Needs a licence.\n' }),
    };
    vm.runInNewContext(readFileSync(join(__dirname, 'imagetpl.js'), 'utf8'), context);

    const editor = factory();
    editor.config = { fields: [], remember: false };
    await editor.download({ target: { action: '/x', querySelector: () => ({}) } });

    assert.equal(notices.length, 1);
    assert.equal(notices[0].description, 'Needs a licence.');
});
