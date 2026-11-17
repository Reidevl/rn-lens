import { StatusBar } from "expo-status-bar";
import { useEffect, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

/**
 * Lens example app.
 *
 * It produces the signals Lens inspects: console output on a timer, a fetch
 * on demand, and an uncaught error on demand. Keep it tiny; it is a fixture
 * for manual verification and for recording protocol fixtures.
 */
export default function App() {
  const [ticks, setTicks] = useState(0);
  const [status, setStatus] = useState<string>("idle");

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
      <Pressable style={styles.button} onPress={fetchJson}>
        <Text style={styles.buttonText}>Fetch JSON</Text>
      </Pressable>
      <Pressable style={[styles.button, styles.danger]} onPress={throwError}>
        <Text style={styles.buttonText}>Throw error</Text>
      </Pressable>
      <StatusBar style="auto" />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#0f172a", alignItems: "center", justifyContent: "center", gap: 12 },
  title: { color: "#f8fafc", fontSize: 24, fontWeight: "600" },
  body: { color: "#94a3b8", fontSize: 16 },
  button: { backgroundColor: "#22c55e", paddingHorizontal: 20, paddingVertical: 12, borderRadius: 8 },
  danger: { backgroundColor: "#ef4444" },
  buttonText: { color: "#0f172a", fontWeight: "600" },
});
