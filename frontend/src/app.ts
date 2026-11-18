import {
  ApiError,
  confirmSeats,
  holdSeats,
  listMovies,
  listSeats,
  listShowtimes,
  releaseSeats,
  type Movie,
  type Seat,
  type SeatHold,
  type Showtime,
} from "./api";

type View = "films" | "times" | "seats" | "receipt";

type HoldState = SeatHold & {
  showtimeId: number;
  seatNumbers: string[];
};

type Receipt = {
  title: string;
  when: string;
  screen: string;
  seatNumbers: string[];
};

const seatPattern = /^([A-Za-z]+)(\d+)$/;

function parseSeat(seatNumber: string): { row: string; col: number } {
  const match = seatPattern.exec(seatNumber);
  if (!match) {
    return { row: seatNumber, col: 0 };
  }
  return { row: match[1].toUpperCase(), col: Number(match[2]) };
}

function formatDuration(minutes: number): string {
  return `${minutes} MIN`;
}

function localDayKey(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

function dayKey(iso: string): string {
  return localDayKey(new Date(iso));
}

function formatDayLabel(iso: string): string {
  const date = new Date(iso);
  const today = new Date();

  if (localDayKey(date) === localDayKey(today)) {
    return "TODAY";
  }

  const tomorrow = new Date(today);
  tomorrow.setDate(today.getDate() + 1);

  if (localDayKey(date) === localDayKey(tomorrow)) {
    return "TOMORROW";
  }

  return new Intl.DateTimeFormat(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
  })
    .format(date)
    .toUpperCase();
}

function formatTime(iso: string): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(iso));
}

function formatDateLine(iso: string): string {
  const date = new Date(iso);

  return new Intl.DateTimeFormat(undefined, {
    weekday: "short",
    day: "numeric",
    month: "short",
    year: "numeric",
  })
    .format(date)
    .toUpperCase();
}

function formatWhen(iso: string): string {
  return `${formatDayLabel(iso)}  ${formatTime(iso)}`;
}

function remainingLabel(expiresAt: string): string {
  const ms = new Date(expiresAt).getTime() - Date.now();

  if (ms <= 0) {
    return "0:00";
  }

  const total = Math.ceil(ms / 1000);
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;

  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

function holdExpired(expiresAt: string): boolean {
  return new Date(expiresAt).getTime() <= Date.now();
}

function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  props: Partial<Record<string, string>> = {},
  text?: string,
): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag);

  for (const [key, value] of Object.entries(props)) {
    if (value !== undefined) {
      element.setAttribute(key, value);
    }
  }

  if (text !== undefined) {
    element.textContent = text;
  }

  return element;
}

export class App {
  private readonly root: HTMLElement;

  private view: View = "films";
  private movies: Movie[] = [];
  private movie: Movie | null = null;
  private showtimes: Showtime[] = [];
  private showtime: Showtime | null = null;
  private seats: Seat[] = [];

  private selectedSeatIds = new Set<number>();
  private hold: HoldState | null = null;
  private receipt: Receipt | null = null;

  private notice = "";
  private busy = false;
  private loading = false;
  private timer: number | null = null;
  private retryable = false;

  private readonly handleVisibility = (): void => this.tick();

  constructor(root: HTMLElement) {
    this.root = root;
    this.root.addEventListener("click", (event) => this.onClick(event));
    document.addEventListener("visibilitychange", this.handleVisibility);
  }

  async start(): Promise<void> {
    this.tick();
    this.timer = window.setInterval(() => this.tick(), 1000);
    await this.loadMovies();
  }

  destroy(): void {
    if (this.timer !== null) {
      window.clearInterval(this.timer);
    }

    document.removeEventListener("visibilitychange", this.handleVisibility);
  }

  private async loadMovies(): Promise<void> {
    this.loading = true;
    this.render();

    try {
      this.movies = await listMovies();
      this.movie = null;
      this.notice =
        this.movies.length === 0 ? "No films in the schedule." : "";
      this.retryable = false;
    } catch (error) {
      this.notice = this.errorText(error);
      this.retryable = true;
    }

    this.loading = false;
    this.render();
  }

  private async openTimes(movie: Movie): Promise<void> {
    this.movie = movie;
    this.view = "times";
    this.notice = "";
    this.loading = true;
    this.render();

    try {
      this.showtimes = await listShowtimes(movie.id);
      this.retryable = false;

      if (this.showtimes.length === 0) {
        this.notice = "No screenings for this film.";
      }
    } catch (error) {
      this.showtimes = [];
      this.notice = this.errorText(error);
      this.retryable = true;
    }

    this.loading = false;
    this.render();
  }

  private async openSeats(showtime: Showtime): Promise<void> {
    // Do not abandon a live hold by silently switching screenings.
    if (this.hold && this.hold.showtimeId !== showtime.id) {
      this.notice = "Release your current seat hold before switching screenings.";
      this.render();
      return;
    }

    this.showtime = showtime;
    this.selectedSeatIds.clear();
    this.seats = [];
    this.view = "seats";
    this.notice = "";
    this.loading = true;
    this.render();

    await this.refreshSeats();

    this.loading = false;
    this.render();
  }

  private async refreshSeats(): Promise<void> {
    if (!this.showtime) {
      return;
    }

    try {
      this.seats = await listSeats(this.showtime.id);
      this.retryable = false;
    } catch (error) {
      this.notice = this.errorText(error);
      this.retryable = true;
    }
  }

  private async onHold(): Promise<void> {
    if (
      !this.showtime ||
      this.busy ||
      this.hold ||
      this.selectedSeatIds.size === 0
    ) {
      return;
    }

    const showtimeId = this.showtime.id;
    const seatIds = [...this.selectedSeatIds];

    const seatNumbers = seatIds.map(
      (id) =>
        this.seats.find((seat) => seat.seat_id === id)?.seat_number ??
        String(id),
    );

    this.busy = true;
    this.notice = "";
    this.render();

    try {
      const held = await holdSeats(showtimeId, seatIds);

      this.hold = {
        ...held,
        showtimeId,
        seatNumbers,
      };

      this.selectedSeatIds.clear();

      await this.refreshSeats();
    } catch (error) {
      this.notice = this.errorText(error);
      await this.refreshSeats();
    } finally {
      this.busy = false;
      this.render();
    }
  }

  private async onConfirm(): Promise<void> {
    if (!this.hold || !this.movie || !this.showtime || this.busy) {
      return;
    }

    this.busy = true;
    this.notice = "";
    this.render();

    const current = this.hold;

    try {
      await confirmSeats(
        current.showtimeId,
        current.seat_ids,
        current.hold_id,
      );

      this.receipt = {
        title: this.movie.title,
        when: formatWhen(this.showtime.starts_at),
        screen: this.showtime.screen_name,
        seatNumbers: current.seatNumbers,
      };

      this.hold = null;
      this.view = "receipt";
    } catch (error) {
      this.notice = this.errorText(error);
      await this.refreshSeats();
    } finally {
      this.busy = false;
      this.render();
    }
  }

  private async onRelease(): Promise<void> {
    if (!this.hold || this.busy) {
      return;
    }

    this.busy = true;
    this.notice = "";
    this.render();

    const current = this.hold;

    try {
      await releaseSeats(
        current.showtimeId,
        current.seat_ids,
        current.hold_id,
      );

      this.hold = null;
    } catch (error) {
      this.notice = this.errorText(error);
    } finally {
      this.busy = false;
      await this.refreshSeats();
      this.render();
    }
  }

  private async expireHold(): Promise<void> {
    this.hold = null;
    this.notice = "Hold ended. Seat map refreshed from the server.";

    if (this.view === "seats") {
      await this.refreshSeats();
    }

    this.render();
  }

  private tick(): void {
    if (this.hold && holdExpired(this.hold.expires_at)) {
      void this.expireHold();
      return;
    }

    if (this.hold && this.view === "seats") {
      const clock = this.root.querySelector("[data-clock]");

      if (clock) {
        clock.textContent = remainingLabel(this.hold.expires_at);
      }
    }
  }

  private errorText(error: unknown): string {
    if (error instanceof ApiError) {
      return error.message;
    }

    if (error instanceof Error) {
      return error.message;
    }

    return "Request failed.";
  }

  private onClick(event: Event): void {
    const target = (event.target as HTMLElement).closest<HTMLElement>(
      "[data-action]",
    );

    if (!target) {
      return;
    }

    const action = target.dataset.action;

    if (action === "select-film") {
      const id = Number(target.dataset.id);
      const movie = this.movies.find((item) => item.id === id);

      if (movie) {
        void this.openTimes(movie);
      }

      return;
    }

    if (action === "open-times") {
      if (this.movie) {
        void this.openTimes(this.movie);
      }

      return;
    }

    if (action === "back-films") {
      this.goHome();
      return;
    }

    if (action === "open-seats") {
      const id = Number(target.dataset.id);
      const showtime = this.showtimes.find((item) => item.id === id);

      if (showtime && !target.hasAttribute("disabled")) {
        void this.openSeats(showtime);
      }

      return;
    }

    if (action === "back-times") {
      if (this.hold) {
        this.notice = "Confirm or release your current hold before leaving.";
        this.render();
        return;
      }

      this.view = "times";
      this.notice = "";
      this.selectedSeatIds.clear();
      this.render();
      return;
    }

    if (action === "select-seat") {
      const seatId = Number(target.dataset.seatId);
      const seat = this.seats.find((item) => item.seat_id === seatId);

      if (
        seat &&
        seat.status === "available" &&
        !this.hold &&
        !this.busy
      ) {
        if (this.selectedSeatIds.has(seatId)) {
          this.selectedSeatIds.delete(seatId);
        } else {
          this.selectedSeatIds.add(seatId);
        }

        this.render();
      }

      return;
    }

    if (action === "hold-selected") {
      void this.onHold();
      return;
    }

    if (action === "confirm") {
      void this.onConfirm();
      return;
    }

    if (action === "release") {
      void this.onRelease();
      return;
    }

    if (action === "home" || action === "brand-home") {
      if (this.hold) {
        this.notice = "Confirm or release your current hold before leaving.";
        this.render();
        return;
      }

      this.goHome();
      return;
    }

    if (action === "retry") {
      if (this.view === "films") {
        void this.loadMovies();
      } else if (this.view === "times" && this.movie) {
        void this.openTimes(this.movie);
      } else if (this.view === "seats") {
        this.notice = "";
        this.loading = true;
        this.render();

        void this.refreshSeats().finally(() => {
          this.loading = false;
          this.render();
        });
      }
    }
  }

  private goHome(): void {
    this.view = "films";
    this.movie = null;
    this.showtime = null;
    this.receipt = null;
    this.notice = "";
    this.retryable = false;
    this.selectedSeatIds.clear();
    this.render();
  }

  private render(): void {
    this.root.replaceChildren();

    const page = document.createElement("div");
    page.className = "page";
    page.append(this.chrome());

    if (this.view === "films") {
      page.append(this.filmsView());
    } else if (this.view === "times") {
      page.append(this.timesView());
    } else if (this.view === "seats") {
      page.append(this.seatsView());
    } else {
      page.append(this.receiptView());
    }

    if (this.notice) {
      const note = document.createElement("p");
      note.className = "notice";
      note.setAttribute("role", "status");
      note.textContent = this.notice;
      page.append(note);

      if (this.retryable) {
        const retry = document.createElement("button");
        retry.type = "button";
        retry.className = "retry-action";
        retry.dataset.action = "retry";
        retry.textContent = "RETRY";
        page.append(retry);
      }
    }

    this.root.append(page);
  }

  private chrome(): HTMLElement {
    const header = document.createElement("header");
    header.className = "chrome";

    const logoWrap = document.createElement("button");
    logoWrap.type = "button";
    logoWrap.className = "logo";
    logoWrap.dataset.action = "brand-home";
    logoWrap.setAttribute(
      "aria-label",
      "LAST ROW — back to film catalogue",
    );

    const mark = document.createElement("span");
    mark.className = "wordmark";
    mark.append(document.createTextNode("LAST ROW"));

    const period = document.createElement("span");
    period.className = "wordmark-period";
    period.textContent = ".";

    mark.append(period);

    const wordmarkGroup = document.createElement("span");
    wordmarkGroup.className = "wordmark-group";
    wordmarkGroup.append(mark);
    logoWrap.append(wordmarkGroup);
    header.append(logoWrap);

    return header;
  }

  private filmsView(): HTMLElement {
    const section = document.createElement("section");
    section.className = "view";

    if (this.loading) {
      const p = document.createElement("p");
      p.className = "loading-text";
      p.textContent = "LOADING";
      section.append(p);
      return section;
    }

    if (this.movies.length === 0) {
      const empty = document.createElement("p");
      empty.className = "empty-state";
      empty.textContent = "There are no films showing right now.";
      section.append(empty);
      return section;
    }

    const intro = document.createElement("div");
    intro.className = "catalogue-intro";

    const meta = document.createElement("div");
    meta.className = "catalogue-meta";

    const label = document.createElement("p");
    label.className = "section-label";
    label.textContent = "NOW SHOWING";

    const count = document.createElement("span");
    count.className = "catalogue-count";
    count.textContent = `${String(this.movies.length).padStart(2, "0")} FILMS`;

    meta.append(label, count);

    const title = document.createElement("h1");
    title.className = "catalogue-title";
    title.textContent = "Choose a film.";

    intro.append(meta, title);
    section.append(intro);

    const list = document.createElement("div");
    list.className = "film-list";
    list.setAttribute("aria-label", "Films currently showing");

    this.movies.forEach((item, index) => {
      const row = document.createElement("button");
      row.type = "button";
      row.className = "film-row";
      row.dataset.action = "select-film";
      row.dataset.id = String(item.id);
      row.setAttribute(
        "aria-label",
        `${item.title}, ${item.duration_minutes} minutes. View showtimes.`,
      );

      const number = document.createElement("span");
      number.className = "film-number";
      number.textContent = String(index + 1).padStart(2, "0");

      const duration = document.createElement("span");
      duration.className = "film-duration";
      duration.textContent = formatDuration(item.duration_minutes);

      const name = document.createElement("span");
      name.className = "film-row-name";
      name.textContent = item.title;

      const action = document.createElement("span");
      action.className = "film-view-times";
      action.textContent = "VIEW SHOWTIMES  →";

      row.append(number, duration, name, action);
      list.append(row);
    });

    section.append(list);
    return section;
  }

  private timesView(): HTMLElement {
    const section = document.createElement("section");
    section.className = "view";

    const back = document.createElement("button");
    back.type = "button";
    back.className = "back";
    back.dataset.action = "back-films";
    back.textContent = "FILMS";
    section.append(back);

    if (this.movie) {
      const title = document.createElement("h1");
      title.className = "poster-title";
      title.textContent = this.movie.title;
      section.append(title);

      const meta = document.createElement("p");
      meta.className = "meta";
      meta.textContent = formatDuration(this.movie.duration_minutes);
      section.append(meta);
    }

    if (this.loading) {
      const p = document.createElement("p");
      p.className = "loading-text";
      p.style.marginTop = "2rem";
      p.textContent = "LOADING";
      section.append(p);
      return section;
    }

    const groups = new Map<string, Showtime[]>();

    for (const show of this.showtimes) {
      const key = dayKey(show.starts_at);
      const bucket = groups.get(key) ?? [];
      bucket.push(show);
      groups.set(key, bucket);
    }

    const now = Date.now();

    for (const [, shows] of groups) {
      const heading = document.createElement("h2");
      heading.className = "day";
      heading.textContent = formatDayLabel(shows[0].starts_at);
      section.append(heading);

      const list = document.createElement("ul");
      list.className = "schedule";

      for (const show of shows) {
        const past = new Date(show.starts_at).getTime() < now;
        const item = document.createElement("li");

        const button = document.createElement("button");
        button.type = "button";
        button.className = "schedule-row";
        button.dataset.action = "open-seats";
        button.dataset.id = String(show.id);

        if (this.showtime?.id === show.id) {
          button.classList.add("is-selected");
        }

        if (past) {
          button.disabled = true;
        }

        button.setAttribute(
          "aria-label",
          `${formatTime(show.starts_at)}, ${show.screen_name}, ${show.remaining_seats} seats remaining${past ? " — past" : ""}`,
        );

        const timeSpan = document.createElement("span");
        timeSpan.className = "num";
        timeSpan.textContent = formatTime(show.starts_at);

        const screenSpan = document.createElement("span");
        screenSpan.textContent = show.screen_name.toUpperCase();

        const seatsSpan = document.createElement("span");
        seatsSpan.className = "num seats-label";
        seatsSpan.textContent = `${show.remaining_seats} LEFT`;

        button.append(timeSpan, screenSpan, seatsSpan);
        item.append(button);
        list.append(item);
      }

      section.append(list);
    }

    if (this.showtimes.length === 0) {
      const empty = document.createElement("p");
      empty.className = "empty-state";
      empty.textContent = "No screenings are scheduled for this film.";
      section.append(empty);
    }

    return section;
  }

  private seatsView(): HTMLElement {
    const section = document.createElement("section");
    section.className = "view";

    const back = document.createElement("button");
    back.type = "button";
    back.className = "back";
    back.dataset.action = "back-times";
    back.textContent = "TIMES";
    section.append(back);

    if (this.movie && this.showtime) {
      const title = document.createElement("h1");
      title.className = "poster-title";
      title.textContent = this.movie.title;
      section.append(title);

      const meta = document.createElement("p");
      meta.className = "meta";
      meta.textContent = [
        formatDateLine(this.showtime.starts_at),
        formatTime(this.showtime.starts_at),
        "–",
        formatTime(this.showtime.ends_at),
        "·",
        this.showtime.screen_name.toUpperCase(),
      ].join("  ");

      section.append(meta);
    }

    const screen = document.createElement("p");
    screen.className = "screen-arc";
    screen.textContent = "SCREEN";
    section.append(screen);

    if (this.loading) {
      const p = document.createElement("p");
      p.className = "loading-text";
      p.textContent = "LOADING";
      section.append(p);
      return section;
    }

    if (this.seats.length === 0) {
      const empty = document.createElement("p");
      empty.className = "empty-state";
      empty.textContent =
        "Seat information is not available for this screening.";
      section.append(empty);
      return section;
    }

    section.append(this.seatMap());
    section.append(this.seatLegend());

    if (!this.hold && this.selectedSeatIds.size > 0) {
      const holdButton = document.createElement("button");
      holdButton.type = "button";
      holdButton.className = "btn-confirm";
      holdButton.dataset.action = "hold-selected";
      holdButton.disabled = this.busy;

      const count = this.selectedSeatIds.size;
      holdButton.textContent = `HOLD ${count} SEAT${count === 1 ? "" : "S"}`;

      section.append(holdButton);
    }

    if (
      this.hold &&
      this.showtime &&
      this.hold.showtimeId === this.showtime.id
    ) {
      section.append(this.holdBar());
    }

    return section;
  }

  private seatMap(): HTMLElement {
    const scroller = document.createElement("div");
    scroller.className = "map-scroll";

    const rows = new Map<string, Seat[]>();
    let maxCol = 0;

    for (const seat of this.seats) {
      const parsed = parseSeat(seat.seat_number);
      maxCol = Math.max(maxCol, parsed.col);

      const bucket = rows.get(parsed.row) ?? [];
      bucket.push(seat);
      rows.set(parsed.row, bucket);
    }

    const columns = Math.max(maxCol, 10);
    const rowNames = [...rows.keys()].sort();

    const table = document.createElement("div");
    table.className = "map";
    table.setAttribute("role", "group");
    table.setAttribute("aria-label", "Seat map");
    table.style.gridTemplateColumns =
      `1.5rem repeat(${columns}, 1.75rem) 1.5rem`;

    const corner = document.createElement("span");
    corner.setAttribute("aria-hidden", "true");
    table.append(corner);

    for (let col = 1; col <= columns; col += 1) {
      const head = document.createElement("span");
      head.className = "axis";
      head.setAttribute("aria-hidden", "true");
      head.textContent = String(col);
      table.append(head);
    }

    table.append(el("span", { "aria-hidden": "true" }));

    for (const row of rowNames) {
      const startLabel = document.createElement("span");
      startLabel.className = "axis";
      startLabel.setAttribute("aria-hidden", "true");
      startLabel.textContent = row;
      table.append(startLabel);

      const byCol = new Map<number, Seat>();

      for (const seat of rows.get(row) ?? []) {
        byCol.set(parseSeat(seat.seat_number).col, seat);
      }

      for (let col = 1; col <= columns; col += 1) {
        const seat = byCol.get(col);

        if (!seat) {
          table.append(el("span", { "aria-hidden": "true" }));
          continue;
        }

        const mine =
          this.hold !== null &&
          this.showtime !== null &&
          this.hold.showtimeId === this.showtime.id &&
          this.hold.seat_ids.includes(seat.seat_id);

        const selected = this.selectedSeatIds.has(seat.seat_id);

        const button = document.createElement("button");
        button.type = "button";
        button.className = "seat";
        button.dataset.seatId = String(seat.seat_id);

        if (mine) {
          button.classList.add("is-mine");
          button.setAttribute(
            "aria-label",
            `Seat ${seat.seat_number}, your hold`,
          );
          button.setAttribute("aria-pressed", "true");
        } else if (seat.status === "booked") {
          button.classList.add("is-booked");
          button.disabled = true;
          button.setAttribute(
            "aria-label",
            `Seat ${seat.seat_number}, booked`,
          );
        } else if (seat.status === "held") {
          button.classList.add("is-held");
          button.disabled = true;
          button.setAttribute(
            "aria-label",
            `Seat ${seat.seat_number}, held`,
          );
        } else if (seat.status !== "available") {
          button.disabled = true;
          button.setAttribute(
            "aria-label",
            `Seat ${seat.seat_number}, unavailable`,
          );
        } else {
          if (selected) {
            button.classList.add("is-selected");
          }

          button.dataset.action = "select-seat";
          button.setAttribute(
            "aria-label",
            `Seat ${seat.seat_number}, ${selected ? "selected" : "available"}`,
          );
          button.setAttribute("aria-pressed", String(selected));
        }

        table.append(button);
      }

      const endLabel = document.createElement("span");
      endLabel.className = "axis";
      endLabel.setAttribute("aria-hidden", "true");
      endLabel.textContent = row;
      table.append(endLabel);
    }

    scroller.append(table);
    return scroller;
  }

  private seatLegend(): HTMLElement {
    const legend = document.createElement("div");
    legend.className = "seat-legend";
    legend.setAttribute("aria-label", "Seat status legend");

    const items: Array<[string, string]> = [
      ["is-available", "Available"],
      ["is-selected", "Selected"],
      ["is-booked", "Booked"],
      ["is-held", "Held"],
      ["is-mine", "Your hold"],
    ];

    for (const [cls, label] of items) {
      const item = document.createElement("span");
      item.className = "legend-item";

      const swatch = document.createElement("span");
      swatch.className = `legend-swatch ${cls}`;
      swatch.setAttribute("aria-hidden", "true");

      const text = document.createElement("span");
      text.textContent = label.toUpperCase();

      item.append(swatch, text);
      legend.append(item);
    }

    return legend;
  }

  private holdBar(): HTMLElement {
    const hold = this.hold!;

    const bar = document.createElement("div");
    bar.className = "hold-bar";

    const summary = document.createElement("p");
    summary.className = "hold-summary";

    const seatStrong = document.createElement("strong");
    seatStrong.className = "hold-seat-num";
    seatStrong.textContent = hold.seatNumbers.join(", ").toUpperCase();

    summary.append(document.createTextNode("Holding seats "), seatStrong);
    bar.append(summary);

    const timer = document.createElement("p");
    timer.className = "hold-timer";

    const clockSpan = document.createElement("span");
    clockSpan.dataset.clock = "";
    clockSpan.textContent = remainingLabel(hold.expires_at);

    timer.append(document.createTextNode("Expires in "), clockSpan);
    bar.append(timer);

    const actions = document.createElement("div");
    actions.className = "actions";

    const confirm = document.createElement("button");
    confirm.type = "button";
    confirm.className = "btn-confirm";
    confirm.dataset.action = "confirm";
    confirm.disabled = this.busy;
    confirm.textContent = "CONFIRM BOOKING";

    const release = document.createElement("button");
    release.type = "button";
    release.className = "btn-ghost";
    release.dataset.action = "release";
    release.disabled = this.busy;
    release.textContent = "RELEASE HOLD";

    actions.append(confirm, release);
    bar.append(actions);

    return bar;
  }

  private receiptView(): HTMLElement {
    const section = document.createElement("section");
    section.className = "view";

    const kicker = document.createElement("p");
    kicker.className = "receipt-kicker";
    kicker.textContent = "BOOKED";
    section.append(kicker);

    if (this.receipt) {
      const title = document.createElement("h1");
      title.className = "poster-title";
      title.textContent = this.receipt.title;
      section.append(title);

      const facts = document.createElement("dl");
      facts.className = "ticket";

      const entries: Array<[string, string]> = [
        ["WHEN", this.receipt.when],
        ["SCREEN", this.receipt.screen.toUpperCase()],
        ["SEATS", this.receipt.seatNumbers.join(", ").toUpperCase()],
      ];

      for (const [label, value] of entries) {
        const row = document.createElement("div");

        const dt = document.createElement("dt");
        dt.textContent = label;

        const dd = document.createElement("dd");
        dd.textContent = value;

        row.append(dt, dd);
        facts.append(row);
      }

      section.append(facts);
    }

    const home = document.createElement("button");
    home.type = "button";
    home.className = "back";
    home.dataset.action = "home";
    home.textContent = "FILMS";

    section.append(home);
    return section;
  }
}