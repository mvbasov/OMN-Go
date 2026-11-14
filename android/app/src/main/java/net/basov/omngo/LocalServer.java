package net.basov.omngo;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.ConnectException;
import java.net.HttpURLConnection;
import java.net.URL;

// ----------------------------------------------------------------------
// The requests of the Android layer to its own server
// ----------------------------------------------------------------------
//
// The Go server of the application listens on 127.0.0.1. A share from
// another application arrives in Java, and Java sends it to the server
// with a POST. This class holds that POST one time.
//
// IT IMPORTS NO android PACKAGE, the same as OmnConfig and OmnText. A
// plain Java virtual machine can thus run its test. See
// android/test/java/net/basov/omngo/LocalServerTest.java.
final class LocalServer {

    // The wait before the one retry of postWithRetry, in milliseconds.
    static final int RETRY_DELAY_MS = 1500;

    private LocalServer() {
    }

    // post sends body to url and answers the text of the answer.
    //
    // Each answer with a code that is not 200 becomes an IOException. Its
    // message is the words of the server. See errorMessage.
    static String post(String url, String contentType, byte[] body) throws IOException {
        HttpURLConnection conn = (HttpURLConnection) new URL(url).openConnection();
        try {
            conn.setRequestMethod("POST");
            conn.setDoOutput(true);
            conn.setRequestProperty("Content-Type", contentType);
            conn.setFixedLengthStreamingMode(body.length);
            OutputStream os = conn.getOutputStream();
            os.write(body);
            os.close();

            int code = conn.getResponseCode();
            InputStream in = (code >= 400) ? conn.getErrorStream() : conn.getInputStream();
            String text = in == null ? "" : OmnText.readAllUtf8(in);
            if (code != 200) {
                throw new IOException(errorMessage(text, code));
            }
            return text;
        } finally {
            conn.disconnect();
        }
    }

    // postWithRetry calls post. When the server refuses the connection, it
    // waits RETRY_DELAY_MS and calls post one more time.
    //
    // A share can arrive during a cold start, and the Go server is then
    // not ready. A refused connection sends nothing, thus the second call
    // cannot write one note two times. Each other fault goes to the caller
    // at once, because the server can hold the data already.
    static String postWithRetry(String url, String contentType, byte[] body) throws IOException {
        try {
            return post(url, contentType, body);
        } catch (ConnectException first) {
            try {
                Thread.sleep(RETRY_DELAY_MS);
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt();
            }
            return post(url, contentType, body);
        }
    }

    // errorMessage answers the words of the server for a refused request.
    // A JSON answer gives its "message". A plain text answer gives its
    // text. An empty answer gives the code.
    static String errorMessage(String text, int code) {
        String trimmed = text == null ? "" : text.trim();
        if (trimmed.startsWith("{")) {
            return OmnText.importErrorMessage(trimmed, code);
        }
        if (!trimmed.isEmpty()) {
            return trimmed;
        }
        return "the server answered HTTP " + code;
    }
}
