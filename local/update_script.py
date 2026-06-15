import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.46"', 'APP_VERSION = "1.0.47"'),
        ("frontend/index.html", 'APP_VERSION = "1.0.46"', 'APP_VERSION = "1.0.47"')
    ]

    for filepath, old_v, new_v in version_replacements:
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if os.path.exists(actual_path):
            with open(actual_path, 'r', encoding='utf-8') as f:
                content = f.read()
            if old_v in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old_v, new_v))
            # Catch scenario where 1.0.46 version bump was skipped or failed
            elif 'APP_VERSION = "1.0.45"' in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace('APP_VERSION = "1.0.45"', new_v))

    # 2. Synchronize Android APK Version in build.gradle
    gradle_path = "android/app/build.gradle" if os.path.exists("android/app/build.gradle") else "backend/android/app/build.gradle"
    
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            content = f.read()

        new_version_code = "10047"
        new_version_name = '"1.0.47"'
        
        content = re.sub(r'versionCode\s+\d+', f'versionCode {new_version_code}', content)
        content = re.sub(r'versionName\s+"[^"]+"', f'versionName {new_version_name}', content)

        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"Successfully synchronized Android APK version in {gradle_path}")

    # 3. Inject Version Footer into HTML
    html_path = "frontend/index.html" if os.path.exists("frontend/index.html") else "backend/frontend/index.html"
    if os.path.exists(html_path):
        with open(html_path, 'r', encoding='utf-8') as f:
            html_content = f.read()
        
        # Inject the footer right before the closing body tag
        if 'goomn-version-footer' not in html_content:
            footer_inject = """
    <!-- Small Version Footer -->
    <div id="goomn-version-footer" style="position: fixed; bottom: 4px; right: 8px; font-size: 0.75rem; color: #888; z-index: 9999; opacity: 0.7; pointer-events: none;"></div>
    <script>
        document.addEventListener("DOMContentLoaded", () => {
            const footer = document.getElementById('goomn-version-footer');
            let v = '1.0.47';
            try { if (APP_VERSION) v = APP_VERSION; } catch(e) {}
            if (footer) footer.innerText = 'GoOMN v' + v;
        });
    </script>
</body>"""
            # Use regex to safely replace </body> regardless of case or whitespace
            html_content = re.sub(r'(?i)</body>', footer_inject, html_content)
            
            with open(html_path, 'w', encoding='utf-8') as f:
                f.write(html_content)
            print(f"Successfully injected version footer into {html_path}")
        else:
            print(f"Version footer already present in {html_path}")

    # 4. Output Standardized Git Commit Message
    commit_msg = """feat(frontend): display app version on a small floating footer
    
- Inject a fixed, unintrusive footer in the bottom right corner of the UI.
- Dynamically bind the footer text to the global APP_VERSION JavaScript variable.
- Synchronize Gradle build manifest and Go server to Version 1.0.47.

Version bumped to 1.0.47"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()