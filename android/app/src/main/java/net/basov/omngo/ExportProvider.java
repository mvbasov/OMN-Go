package net.basov.omngo;

/**
 * Hands ONE exported note to another application.
 *
 * WHY THIS EXISTS AND NOT androidx.core.content.FileProvider.
 *
 * OMN-Go builds against the platform SDK and nothing else - the whole
 * dependency block of app/build.gradle is a fileTree over an empty libs/ -
 * and FileProvider lives in AndroidX. Rather than take that dependency for
 * one screen's worth of behaviour, this is the part of it the share sheet
 * needs: read one file, say what it is called and how large it is.
 *
 * WHY NOT A file:// URI. MainActivity clears the StrictMode VM policy for
 * the external-editor path, so a file:// URI would not throw
 * FileUriExposedException here either. It would still be the wrong answer: a
 * receiving application on a modern Android cannot READ a path under another
 * application's external media directory, so Telegram or a mail client would
 * take the share and attach nothing. A content:// URI with a read grant is
 * the mechanism the platform actually supports.
 *
 * SCOPE. The provider serves cacheDir/export and no other directory, one
 * path segment deep, read-only, and it is exported="false" in the manifest -
 * nothing can reach it except an application this one hands a URI to, and
 * only for as long as that grant lives. The note tree itself is never
 * exposed.
 */
public class ExportProvider extends android.content.ContentProvider {

    /** The one directory this provider serves. Created on demand. */
    static java.io.File exportDir(android.content.Context ctx) {
        java.io.File dir = new java.io.File(ctx.getCacheDir(), "export");
        dir.mkdirs();
        return dir;
    }

    /**
     * The authority, which must match android:authorities in the manifest.
     *
     * getPackageName() is the applicationId, so this follows the product
     * flavor: the standard build and the F-Droid build are installable side
     * by side and must not claim the same authority.
     */
    static String authority(android.content.Context ctx) {
        return ctx.getPackageName() + ".export";
    }

    static android.net.Uri uriFor(android.content.Context ctx, java.io.File file) {
        return new android.net.Uri.Builder()
                .scheme("content")
                .authority(authority(ctx))
                .appendPath(file.getName())
                .build();
    }

    /**
     * Resolves a URI to a file inside the export directory, or refuses.
     *
     * The containment check is on the CANONICAL path. A single path segment
     * cannot hold a "/", but it can hold "..", and the question worth asking
     * is where the path ends up rather than what it looks like.
     */
    private java.io.File resolve(android.net.Uri uri) throws java.io.FileNotFoundException {
        java.util.List<String> segments = uri.getPathSegments();
        if (segments == null || segments.size() != 1) {
            throw new java.io.FileNotFoundException("not an export URI: " + uri);
        }
        java.io.File dir = exportDir(getContext());
        java.io.File file = new java.io.File(dir, segments.get(0));
        try {
            String root = dir.getCanonicalPath() + java.io.File.separator;
            if (!file.getCanonicalPath().startsWith(root)) {
                throw new java.io.FileNotFoundException("outside the export directory: " + uri);
            }
        } catch (java.io.IOException e) {
            throw new java.io.FileNotFoundException("cannot resolve: " + uri);
        }
        if (!file.isFile()) {
            throw new java.io.FileNotFoundException("no such export: " + uri);
        }
        return file;
    }

    @Override
    public boolean onCreate() {
        return true;
    }

    @Override
    public android.os.ParcelFileDescriptor openFile(android.net.Uri uri, String mode)
            throws java.io.FileNotFoundException {
        // Read-only, whatever was asked for. Nothing outside this application
        // has a reason to write into the export cache.
        if (!"r".equals(mode)) {
            throw new java.io.FileNotFoundException("read only: " + uri);
        }
        return android.os.ParcelFileDescriptor.open(resolve(uri),
                android.os.ParcelFileDescriptor.MODE_READ_ONLY);
    }

    /**
     * DISPLAY_NAME and SIZE. A mail client asks for both before it will show
     * an attachment, and Telegram asks for the name; a provider that answers
     * neither produces a share that looks empty.
     */
    @Override
    public android.database.Cursor query(android.net.Uri uri, String[] projection,
            String selection, String[] selectionArgs, String sortOrder) {
        java.io.File file;
        try {
            file = resolve(uri);
        } catch (java.io.FileNotFoundException e) {
            return null;
        }
        String[] asked = projection;
        if (asked == null) {
            asked = new String[] {
                android.provider.OpenableColumns.DISPLAY_NAME,
                android.provider.OpenableColumns.SIZE
            };
        }
        java.util.ArrayList<String> columns = new java.util.ArrayList<String>();
        java.util.ArrayList<Object> values = new java.util.ArrayList<Object>();
        for (String column : asked) {
            if (android.provider.OpenableColumns.DISPLAY_NAME.equals(column)) {
                columns.add(column);
                values.add(file.getName());
            } else if (android.provider.OpenableColumns.SIZE.equals(column)) {
                columns.add(column);
                values.add(Long.valueOf(file.length()));
            }
        }
        android.database.MatrixCursor cursor = new android.database.MatrixCursor(
                columns.toArray(new String[columns.size()]), 1);
        cursor.addRow(values.toArray());
        return cursor;
    }

    @Override
    public String getType(android.net.Uri uri) {
        return "text/markdown";
    }

    @Override
    public android.net.Uri insert(android.net.Uri uri, android.content.ContentValues values) {
        throw new UnsupportedOperationException("the export cache is read only");
    }

    @Override
    public int delete(android.net.Uri uri, String selection, String[] selectionArgs) {
        throw new UnsupportedOperationException("the export cache is read only");
    }

    @Override
    public int update(android.net.Uri uri, android.content.ContentValues values,
            String selection, String[] selectionArgs) {
        throw new UnsupportedOperationException("the export cache is read only");
    }
}
