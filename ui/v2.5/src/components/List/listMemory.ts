import type { SortDirectionEnum } from "src/core/generated-graphql";
import type { DisplayMode } from "src/models/list-filter/types";

// Remembers the last state of each list page in local storage, per user.

export interface IListSettings {
  sortBy?: string;
  sortDirection: SortDirectionEnum;
  itemsPerPage: number;
  displayMode: DisplayMode;
  zoomIndex: number;
}

export interface IListCriteria {
  searchTerm: string;
  // each criterion as encoded in the URL query
  criteria: string[];
}

export interface IListState {
  settings: IListSettings;
  criteria: IListCriteria;
}

// the parts are saved independently, so either may be missing
export type RememberedListState = Partial<IListState>;

interface IListMemorySlot {
  storageKey: string;
  page: string;
}

type ListMemoryStorage = Pick<Storage, "getItem" | "setItem">;

const maxRememberedPages = 500;

const storageKeyPrefix = "stash-list-memory";

// returns undefined for lists without a view, and while the current user
// could not be loaded
export function listMemorySlot(props: {
  view?: string;
  pathname: string;
  userName?: string;
  loadFailed?: boolean;
}): IListMemorySlot | undefined {
  const { view, pathname, userName, loadFailed } = props;
  if (!view || loadFailed) return;

  const storageKey =
    userName === undefined
      ? storageKeyPrefix
      : `${storageKeyPrefix}:${userName}`;

  // lists on an entity's own page, e.g. /performers/123/scenes, are
  // remembered per entity
  const entityID = pathname.split("/")[2];
  const page =
    entityID && /^\d+$/.test(entityID) ? `${view}:${entityID}` : view;

  return { storageKey, page };
}

export function browserStorage(): ListMemoryStorage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

// the pages of a user's memory, least recently saved first
function loadPages(storage: ListMemoryStorage, storageKey: string) {
  const stored = storage.getItem(storageKey);
  try {
    if (stored) {
      return new Map<string, RememberedListState>(JSON.parse(stored));
    }
  } catch {
    // ignore - an unparseable memory is replaced by the next save
  }
  return new Map<string, RememberedListState>();
}

function storePages(
  storage: ListMemoryStorage,
  storageKey: string,
  pages: Map<string, RememberedListState>
) {
  while (pages.size > maxRememberedPages) {
    pages.delete(pages.keys().next().value!);
  }
  storage.setItem(storageKey, JSON.stringify([...pages]));
}

export function readListMemory(
  storage: ListMemoryStorage | undefined,
  slot: IListMemorySlot | undefined
): RememberedListState | undefined {
  if (!storage || !slot) return;

  try {
    return loadPages(storage, slot.storageKey).get(slot.page);
  } catch {
    // ignore - storage may be unavailable
  }
}

// each remembered part takes precedence over the same part of the defaults
export function mergeListState(
  defaults: IListState,
  remembered: RememberedListState | undefined
): IListState {
  return {
    settings: remembered?.settings ?? defaults.settings,
    criteria: remembered?.criteria ?? defaults.criteria,
  };
}

function sameSettings(a: IListSettings, b: IListSettings) {
  return (
    a.sortBy === b.sortBy &&
    a.sortDirection === b.sortDirection &&
    a.itemsPerPage === b.itemsPerPage &&
    a.displayMode === b.displayMode &&
    a.zoomIndex === b.zoomIndex
  );
}

function sameCriteria(a: IListCriteria, b: IListCriteria) {
  return (
    a.searchTerm === b.searchTerm &&
    a.criteria.length === b.criteria.length &&
    a.criteria.every((c, i) => c === b.criteria[i])
  );
}

// Only the changed parts are saved, so criteria that came with a link are not
// remembered unless the user edits them.
export function saveListMemory(
  storage: ListMemoryStorage | undefined,
  slot: IListMemorySlot | undefined,
  prev: IListState,
  next: IListState
) {
  if (!storage || !slot) return;

  const settingsChanged = !sameSettings(prev.settings, next.settings);
  const criteriaChanged = !sameCriteria(prev.criteria, next.criteria);
  if (!settingsChanged && !criteriaChanged) return;

  try {
    const pages = loadPages(storage, slot.storageKey);
    const remembered = { ...pages.get(slot.page) };
    if (settingsChanged) remembered.settings = next.settings;
    if (criteriaChanged) remembered.criteria = next.criteria;

    // move the page to the end, as the most recently saved
    pages.delete(slot.page);
    pages.set(slot.page, remembered);
    storePages(storage, slot.storageKey, pages);
  } catch {
    // ignore - storage may be unavailable or full
  }
}
