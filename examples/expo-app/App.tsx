import { useLens } from "@rnlens/sdk";
import {
  QueryClient,
  QueryClientProvider,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { StatusBar } from "expo-status-bar";
import { useEffect, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import { seedStorage, storage } from "./storage";

/**
 * Lens example app.
 *
 * It produces the signals Lens inspects: console output on a timer, a fetch
 * on demand, an uncaught error on demand, and TanStack Query traffic (two
 * queries and one mutation) streamed to Lens through @rnlens/sdk, and three
 * key-value stores (AsyncStorage, MMKV in a development build, SecureStore)
 * for the Storage panel.
 */
const queryClient = new QueryClient();

let failNext = false;

async function fetchTodos(): Promise<Array<{ id: number; title: string }>> {
  await new Promise((r) => setTimeout(r, 400));
  if (failNext) {
    failNext = false;
    throw new Error("Simulated server error");
  }
  return [
    { id: 1, title: "Attach Lens" },
    { id: 2, title: "Inspect queries" },
  ];
}

async function fetchProfile(): Promise<{ name: string; plan: string; updatedAt: string }> {
  await new Promise((r) => setTimeout(r, 1500));
  return { name: "Ada", plan: "pro", updatedAt: new Date().toISOString() };
}

function Demo() {
  const client = useQueryClient();
  const [ticks, setTicks] = useState(0);
  const [status, setStatus] = useState<string>("idle");
  const todos = useQuery({ queryKey: ["todos"], queryFn: fetchTodos, staleTime: 10_000 });
  const profile = useQuery({
    queryKey: ["profile", { id: 1 }],
    queryFn: fetchProfile,
    staleTime: 60_000,
  });
  const addTodo = useMutation({
    mutationKey: ["addTodo"],
    mutationFn: async (title: string) => {
      await new Promise((r) => setTimeout(r, 300));
      return { id: Date.now(), title };
    },
    onSuccess: () => client.invalidateQueries({ queryKey: ["todos"] }),
  });

  useEffect(() => {
    const id = setInterval(() => {
      setTicks((n) => {
        const next = n + 1;
        console.log("tick", { n: next, at: new Date().toISOString() });
        return next;
      });
    }, 2000);
    return () => clearInterval(id);
  }, []);

  const fetchJson = async () => {
    setStatus("fetching");
    try {
      const res = await fetch("https://httpbin.org/json");
      const body = await res.json();
      console.info("fetched", body.slideshow?.title);
      setStatus(`ok ${res.status}`);
    } catch (err) {
      console.error("fetch failed", err);
      setStatus("error");
    }
  };

  const throwError = () => {
    throw new Error("Lens example: intentional error");
  };

  return (
    <View style={styles.container}>
      <Text style={styles.title}>Lens example</Text>
      <Text style={styles.body}>ticks: {ticks}</Text>
      <Text style={styles.body}>fetch: {status}</Text>
      <Text style={styles.body}>
        todos: {todos.status}/{todos.fetchStatus} ({todos.data?.length ?? 0})
      </Text>
      <Text style={styles.body}>
        profile: {profile.status} {profile.data?.name ?? ""}
      </Text>
      <Pressable style={styles.button} onPress={fetchJson}>
        <Text style={styles.buttonText}>Fetch JSON</Text>
      </Pressable>
      <Pressable style={styles.button} onPress={() => addTodo.mutate(`Todo ${Date.now() % 1000}`)}>
        <Text style={styles.buttonText}>Add todo (mutation)</Text>
      </Pressable>
      <Pressable
        style={styles.button}
        onPress={() => {
          failNext = true;
          void client.invalidateQueries({ queryKey: ["todos"] });
        }}
      >
        <Text style={styles.buttonText}>Refetch todos with error</Text>
      </Pressable>
      <Pressable style={[styles.button, styles.danger]} onPress={throwError}>
        <Text style={styles.buttonText}>Throw error</Text>
      </Pressable>
      <StatusBar style="auto" />
    </View>
  );
}

export default function App() {
  // EXPO_PUBLIC_LENS_URL points the SDK at a Lens that is not on the default
  // port (the e2e suite uses it); unset, the host comes from the Metro bundle.
  useLens({ queryClient, storage, appName: "Lens Example", url: process.env.EXPO_PUBLIC_LENS_URL });
  useEffect(() => {
    seedStorage().catch((err: unknown) => console.warn("seeding storage failed", err));
  }, []);
  return (
    <QueryClientProvider client={queryClient}>
      <Demo />
    </QueryClientProvider>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#0f172a",
    alignItems: "center",
    justifyContent: "center",
    gap: 12,
  },
  title: { color: "#f8fafc", fontSize: 24, fontWeight: "600" },
  body: { color: "#94a3b8", fontSize: 16 },
  button: {
    backgroundColor: "#22c55e",
    paddingHorizontal: 20,
    paddingVertical: 12,
    borderRadius: 8,
  },
  danger: { backgroundColor: "#ef4444" },
  buttonText: { color: "#0f172a", fontWeight: "600" },
});
