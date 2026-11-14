package net.basov.omngo;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.ServerSocket;

// ----------------------------------------------------------------------
// The test of LocalServer
// ----------------------------------------------------------------------
//
// It starts a small HTTP server of the JDK on 127.0.0.1 and sends real
// requests to it. The banner of OmnConfigTest says why this directory is
// outside the Gradle project, and why the test uses no JUnit.
//
// TestJavaUnitTests in backend/internal/repocheck/java_test.go compiles and
// runs this file.
public final class LocalServerTest {

    private static int failures = 0;
    private static int requests = 0;
    private static String lastBody = "";
    private static String lastType = "";

    public static void main(String[] args) throws Exception {
        HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", new HttpHandler() {
            @Override
            public void handle(HttpExchange ex) throws IOException {
                requests++;
                lastBody = OmnText.readAllUtf8(ex.getRequestBody());
                lastType = ex.getRequestHeaders().getFirst("Content-Type");
                String path = ex.getRequestURI().getPath();
                int code = 200;
                String answer = "Saved";
                if ("/json-error".equals(path)) {
                    code = 400;
                    answer = "{\"status\":\"error\",\"message\":\"the file is too large\"}";
                } else if ("/text-error".equals(path)) {
                    code = 500;
                    answer = "Failed to save\n";
                } else if ("/empty-error".equals(path)) {
                    code = 503;
                    answer = "";
                }
                byte[] out = answer.getBytes("UTF-8");
                ex.sendResponseHeaders(code, out.length == 0 ? -1 : out.length);
                if (out.length > 0) {
                    ex.getResponseBody().write(out);
                }
                ex.close();
            }
        });
        server.start();
        String base = "http://127.0.0.1:" + server.getAddress().getPort();
        try {
            aGoodPost(base);
            theWordsOfTheServer(base);
            noRetryAfterAnAnswer(base);
        } finally {
            server.stop(0);
        }
        aRefusedConnection();

        if (failures > 0) {
            System.out.println(failures + " check(s) failed");
            System.exit(1);
        }
        System.out.println("LocalServerTest: every check passed");
    }

    // A post sends the body and the content type, and answers the text.
    private static void aGoodPost(String base) throws Exception {
        String answer = LocalServer.post(base + "/api/quick", "text/plain; charset=utf-8", "note=a".getBytes("UTF-8"));
        eq("the answer", "Saved", answer);
        eq("the body", "note=a", lastBody);
        eq("the content type", "text/plain; charset=utf-8", lastType);
    }

    // A refused request gives the words of the server. A person reads
    // them in a toast, and "HTTP 500" alone says nothing.
    private static void theWordsOfTheServer(String base) {
        eq("a JSON error", "the file is too large", messageOf(base + "/json-error"));
        eq("a text error", "Failed to save", messageOf(base + "/text-error"));
        eq("an empty error", "the server answered HTTP 503", messageOf(base + "/empty-error"));
    }

    // An answer of the server stops the retry. The server can hold the
    // data already, and a second post would write one note two times.
    private static void noRetryAfterAnAnswer(String base) {
        int before = requests;
        try {
            LocalServer.postWithRetry(base + "/text-error", "text/plain", new byte[0]);
            eq("a refusal throws", "an exception", "no exception");
        } catch (IOException e) {
            eq("the message", "Failed to save", e.getMessage());
        }
        eq("the requests of one refused post", 1, requests - before);
    }

    // A closed port refuses the connection. postWithRetry waits, tries one
    // more time, and then gives the fault to the caller.
    private static void aRefusedConnection() throws Exception {
        ServerSocket free = new ServerSocket(0);
        int port = free.getLocalPort();
        free.close();
        long start = System.currentTimeMillis();
        try {
            LocalServer.postWithRetry("http://127.0.0.1:" + port + "/x", "text/plain", new byte[0]);
            eq("a closed port throws", "an exception", "no exception");
        } catch (java.net.ConnectException e) {
            long waited = System.currentTimeMillis() - start;
            eq("the retry waited", true, waited >= LocalServer.RETRY_DELAY_MS);
        }
    }

    private static String messageOf(String url) {
        try {
            LocalServer.post(url, "text/plain", new byte[0]);
            return "no exception";
        } catch (IOException e) {
            return e.getMessage();
        }
    }

    private static void eq(String what, Object want, Object got) {
        if (!want.equals(got)) {
            failures++;
            System.out.println("FAIL " + what + ": want " + want + ", got " + got);
        }
    }
}
