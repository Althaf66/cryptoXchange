"use client";

import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { getVirtualUsers } from "../utils/httpClient";
import { VirtualUser } from "../utils/types";

/**
 * Demo mode: the engine seeds accounts with balances on first boot, so the whole
 * trading flow is exercisable without a login. The list comes from the engine -
 * see VirtualUsers on the home page for adding to it. Signup/login exist on the
 * API and are untouched.
 */
const STORAGE_KEY = "cryptoxchange:userId";

/** The first account ensureMarkets seeds. Used until the fetch lands, since
 *  Balances and OpenOrders call the API on their first render. */
const FALLBACK_USER_ID = "1";

interface UserContextValue {
    userId: string;
    setUserId: (id: string) => void;
    users: VirtualUser[];
    refreshUsers: () => Promise<VirtualUser[]>;
}

const UserContext = createContext<UserContextValue>({
    userId: FALLBACK_USER_ID,
    setUserId: () => {},
    users: [],
    refreshUsers: async () => [],
});

export function UserProvider({ children }: { children: React.ReactNode }) {
    const [userId, setUserIdState] = useState(FALLBACK_USER_ID);
    const [users, setUsers] = useState<VirtualUser[]>([]);

    const refreshUsers = useCallback(async () => {
        const fetched = await getVirtualUsers();
        setUsers(fetched);
        return fetched;
    }, []);

    // Read after mount: localStorage does not exist during the server render.
    useEffect(() => {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (stored) setUserIdState(stored);

        refreshUsers()
            .then((fetched) => {
                // A stored id can outlive its account - snapshot.json gets deleted.
                if (fetched.length && !fetched.some((u) => u.id === (stored ?? FALLBACK_USER_ID))) {
                    setUserId(fetched[0].id);
                }
            })
            .catch((err) => console.error("Failed to fetch users:", err));
    }, [refreshUsers]);

    const setUserId = (id: string) => {
        setUserIdState(id);
        localStorage.setItem(STORAGE_KEY, id);
    };

    return (
        <UserContext.Provider value={{ userId, setUserId, users, refreshUsers }}>
            {children}
        </UserContext.Provider>
    );
}

export function useUser() {
    return useContext(UserContext);
}
