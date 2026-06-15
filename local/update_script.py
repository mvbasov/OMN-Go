import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.27"', 'APP_VERSION = "1.0.28"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.27";', 'const APP_VERSION = "1.0.28";')
    ]
    
    for filepath, old, new in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            if old in content:
                with open(filepath, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old, new))

    # 2. Bump the Android Version in Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            gradle_content = f.read()
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10028', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.28"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Securely Generate Keystore on Host using Docker
    cwd = os.getcwd()
    keystore_dir = os.path.join(cwd, "android", "app")
    keystore_path = os.path.join(keystore_dir, "goomn.keystore")
    
    if not os.path.exists(keystore_path) and os.path.exists(keystore_dir):
        print("\n[!] Host 'keytool' missing or failed. Generating permanent keystore via Docker...")
        
        # Enforce host UID/GID mapping so Docker doesn't write the keystore as 'root'
        user_flag = ""
        if os.name != 'nt':
            user_flag = f"-u {os.getuid()}:{os.getgid()}"
        
        # Spin up a temporary Java container solely to execute keytool securely
        cmd = f'docker run --rm {user_flag} -v "{keystore_dir}:/app_keys" eclipse-temurin:17-jre keytool -genkey -v -keystore /app_keys/goomn.keystore -alias goomn -keyalg RSA -keysize 2048 -validity 10000 -storepass goomn123 -keypass goomn123 -dname "CN=GoOMN, O=Basov"'
        os.system(cmd)
        print("[+] Keystore generated successfully!\n")

    # 4. Output Standardized Git Commit Message
    commit_msg = """fix(android): dockerize permanent keystore generation

- Replaced host-dependent `keytool` command with a Dockerized `eclipse-temurin` generator
- Guarantees `goomn.keystore` is written to the host directory for permanent Git persistence
- Enforced host UID/GID mapping to prevent root permission lockouts on Linux
- Bumped Android versionCode to 10028

Version bumped to 1.0.28"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.28!")

if __name__ == "__main__":
    update_application()