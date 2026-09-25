// Iridium Image Template client state for live preview and browser-local drafts.
// The server remains authoritative for the exported image; this component only
// keeps the editor responsive and restores the last non-sensitive draft.
(function () {
    const COMPONENT_NAME = 'imagetpl';
    const MAX_TEXT_LENGTH = 2000;
    const FONT_STYLES = {
        regular: { fontFamily: 'system-ui, -apple-system, sans-serif', fontWeight: '400', fontStyle: 'normal' },
        bold: { fontFamily: 'system-ui, -apple-system, sans-serif', fontWeight: '700', fontStyle: 'normal' },
        italic: { fontFamily: 'system-ui, -apple-system, sans-serif', fontWeight: '400', fontStyle: 'italic' },
        'bold-italic': { fontFamily: 'system-ui, -apple-system, sans-serif', fontWeight: '700', fontStyle: 'italic' },
        mono: { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontWeight: '400', fontStyle: 'normal' },
    };

    const validColour = (value) => /^#[0-9a-f]{6}$/i.test(value || '');
    const limitedText = (value) => Array.from(value || '').slice(0, MAX_TEXT_LENGTH).join('');

    const componentFactory = () => ({
        config: null,
        fields: {},
        styles: {},
        saveMessage: '',
        suppressPersistence: false,

        init() {
            this.config = this.readConfig();
            const defaults = this.config.fieldDefaults || {};
            const styleDefaults = this.config.styleDefaults || {};
            const fields = this.config.fields || Object.keys(defaults);

            fields.forEach((field) => {
                this.fields[field] = limitedText(defaults[field] || '');
                const style = styleDefaults[field] || {};
                this.styles[field] = {
                    font: style.font || this.config.fonts?.[0]?.id || 'regular',
                    color: validColour(style.color) ? style.color.toLowerCase() : '#ffffff',
                };
            });

            if (this.config.remember) {
                this.restore();
                this.$watch('fields', () => this.persist(), { deep: true });
                this.$watch('styles', () => this.persist(), { deep: true });
            }
        },

        readConfig() {
            try {
                return JSON.parse(this.$el.dataset.config || '{}');
            } catch (_) {
                return { fields: [], fieldDefaults: {}, styleDefaults: {}, fonts: [], remember: false };
            }
        },

        allowedFont(id) {
            return (this.config.fonts || []).some((font) => font.id === id);
        },

        restore() {
            try {
                const raw = window.localStorage.getItem(this.config.storageKey);
                if (!raw) return;
                const saved = JSON.parse(raw);
                const fields = this.config.fields || [];

                fields.forEach((field) => {
                    if (typeof saved.fields?.[field] === 'string') {
                        this.fields[field] = limitedText(saved.fields[field]);
                    }
                    const style = saved.styles?.[field];
                    if (!style) return;
                    if (typeof style.font === 'string' && this.allowedFont(style.font)) {
                        this.styles[field].font = style.font;
                    }
                    if (typeof style.color === 'string' && validColour(style.color)) {
                        this.styles[field].color = style.color.toLowerCase();
                    }
                });
                this.saveMessage = 'Restored from this browser';
            } catch (_) {
                this.saveMessage = 'Draft saving is unavailable';
            }
        },

        persist() {
            if (this.suppressPersistence || !this.config?.remember) return;
            try {
                window.localStorage.setItem(this.config.storageKey, JSON.stringify({
                    version: 1,
                    fields: this.fields,
                    styles: this.styles,
                }));
                this.saveMessage = 'Saved automatically in this browser';
            } catch (_) {
                this.saveMessage = 'Draft saving is unavailable';
            }
        },

        async download(event) {
            this.persist();
            const form = event.target;
            const button = form.querySelector('button[type="submit"]');
            if (button) button.disabled = true;
            try {
                const token = window.Alpine?.store('security')?.csrf || '';
                const response = await fetch(form.action, {
                    method: 'POST',
                    body: new FormData(form),
                    credentials: 'same-origin',
                    headers: { 'X-CSRF-Token': token },
                });
                if (!response.ok) throw new Error('The JPEG could not be created.');
                const data = await response.blob();
                const disposition = response.headers.get('Content-Disposition') || '';
                const filename = disposition.match(/filename="?([^";]+)"?/i)?.[1] || 'template.jpg';
                const url = URL.createObjectURL(data);
                const link = document.createElement('a');
                link.href = url;
                link.download = filename;
                document.body.append(link);
                link.click();
                link.remove();
                setTimeout(() => URL.revokeObjectURL(url), 60000);
            } catch (error) {
                window.dispatchEvent(new CustomEvent('notify', { detail: {
                    type: 'error', message: 'Download failed', description: error.message,
                } }));
            } finally {
                if (button) button.disabled = false;
            }
        },

        clearSaved() {
            this.suppressPersistence = true;
            try {
                window.localStorage.removeItem(this.config.storageKey);
            } catch (_) {
                // The form still resets if browser storage is unavailable.
            }
            const defaults = this.config.fieldDefaults || {};
            const styleDefaults = this.config.styleDefaults || {};
            (this.config.fields || []).forEach((field) => {
                this.fields[field] = limitedText(defaults[field] || '');
                const style = styleDefaults[field] || {};
                this.styles[field] = {
                    font: style.font || this.config.fonts?.[0]?.id || 'regular',
                    color: validColour(style.color) ? style.color.toLowerCase() : '#ffffff',
                };
            });
            this.suppressPersistence = false;
            this.saveMessage = 'Saved values cleared';
        },

        styleFor(field) {
            const style = this.styles[field] || {};
            const font = FONT_STYLES[style.font] || FONT_STYLES.regular;
            return {
                color: validColour(style.color) ? style.color : '#ffffff',
                fontFamily: font.fontFamily,
                fontWeight: font.fontWeight,
                fontStyle: font.fontStyle,
            };
        },
    });

    const register = () => {
        if (!window.Alpine) return false;
        window.Alpine.data(COMPONENT_NAME, componentFactory);
        return true;
    };

    if (!register()) {
        document.addEventListener('alpine:init', register, { once: true });
    }
})();
