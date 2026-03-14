import { initializeApp } from "firebase/app"
import { getMessaging } from "firebase/messaging"

const firebaseConfig = {
  apiKey: "AIzaSyDwAeORMe8Cyx9ewIzLF2g3qJSa6qRPaBc",
  authDomain: "blog-but-different.firebaseapp.com",
  projectId: "blog-but-different",
  storageBucket: "blog-but-different.firebasestorage.app",
  messagingSenderId: "572427222580",
  appId: "1:572427222580:web:fddb92bbb43a53e893d461",
}

export const VAPID_KEY = "BOdTgrB8lEeZxwfGfSraKzvDE2aDJ223O8ObycFJdGbDLvi-PggL_TEjHX8RtQaoQw9UGbI0dxWpuXF1B4jYukA"

const app = initializeApp(firebaseConfig)
export const messaging = getMessaging(app)
