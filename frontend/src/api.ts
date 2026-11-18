const API_BASE =
  (import.meta.env.VITE_API_URL as string | undefined) ??
  "http://localhost:8000";

export type Movie = {
  id: number;
  title: string;
  duration_minutes: number;
};

export type Showtime = {
  id: number;
  screen_name: string;
  starts_at: string;
  ends_at: string;
  remaining_seats: number;
};

export type Seat = {
  seat_id: number;
  seat_number: string;
  status: "available" | "held" | "booked" | string;
};

export type SeatHold = {
  hold_id: string;
  seat_ids: number[];
  expires_at: string;
};

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function parseError(response: Response): Promise<ApiError> {
  const text = (await response.text()).trim();
  return new ApiError(response.status, text || response.statusText);
}

async function getJson<T>(path: string): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`);

  if (!response.ok) {
    throw await parseError(response);
  }

  return (await response.json()) as T;
}

async function postJson<T>(
  path: string,
  body: unknown,
): Promise<{ status: number; data: T }> {
  const response = await fetch(`${API_BASE}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    throw await parseError(response);
  }

  return {
    status: response.status,
    data: (await response.json()) as T,
  };
}

export async function listMovies(): Promise<Movie[]> {
  const movies = await getJson<Movie[] | null>("/movies");
  return movies ?? [];
}

export async function listShowtimes(movieId: number): Promise<Showtime[]> {
  const showtimes = await getJson<Showtime[] | null>(
    `/movies/${movieId}/showtimes`,
  );
  return showtimes ?? [];
}

export async function listSeats(showtimeId: number): Promise<Seat[]> {
  const seats = await getJson<Seat[] | null>(`/showtimes/${showtimeId}/seats`);
  return seats ?? [];
}

export async function holdSeats(
  showtimeId: number,
  seatIds: number[],
): Promise<SeatHold> {
  const { data } = await postJson<SeatHold>(
    `/showtimes/${showtimeId}/seats/hold`,
    { seat_ids: seatIds },
  );

  return data;
}

export async function confirmSeats(
  showtimeId: number,
  seatIds: number[],
  holdId: string,
): Promise<void> {
  await postJson(`/showtimes/${showtimeId}/seats/confirm`, {
    seat_ids: seatIds,
    hold_id: holdId,
  });
}

export async function releaseSeats(
  showtimeId: number,
  seatIds: number[],
  holdId: string,
): Promise<void> {
  await postJson(`/showtimes/${showtimeId}/seats/release`, {
    seat_ids: seatIds,
    hold_id: holdId,
  });
}
