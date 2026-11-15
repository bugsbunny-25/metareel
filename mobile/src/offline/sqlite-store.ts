import * as SQLite from 'expo-sqlite';

import type { OutboxItem, OutboxStore } from './outbox';

/** The outbox in its own SQLite database, so it survives app kills and restarts. */
export class SQLiteStore implements OutboxStore {
  private db = SQLite.openDatabaseSync('metareel-outbox.db');

  constructor() {
    this.db.execSync(`CREATE TABLE IF NOT EXISTS outbox (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      item TEXT NOT NULL
    )`);
  }

  insert(item: Omit<OutboxItem, 'id'>): number {
    const r = this.db.runSync('INSERT INTO outbox (item) VALUES (?)', JSON.stringify(item));
    return Number(r.lastInsertRowId);
  }

  all(): OutboxItem[] {
    return this.db
      .getAllSync<{ id: number; item: string }>('SELECT id, item FROM outbox ORDER BY id')
      .map((r) => ({ ...(JSON.parse(r.item) as Omit<OutboxItem, 'id'>), id: r.id }));
  }

  update(id: number, patch: Partial<Omit<OutboxItem, 'id'>>): void {
    const row = this.db.getFirstSync<{ item: string }>('SELECT item FROM outbox WHERE id = ?', id);
    if (!row) return;
    this.db.runSync('UPDATE outbox SET item = ? WHERE id = ?', JSON.stringify({ ...JSON.parse(row.item), ...patch }), id);
  }

  remove(id: number): void {
    this.db.runSync('DELETE FROM outbox WHERE id = ?', id);
  }

  clear(): void {
    this.db.runSync('DELETE FROM outbox');
  }
}
