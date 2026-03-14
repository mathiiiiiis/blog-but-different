importScripts('https://www.gstatic.com/firebasejs/10.14.1/firebase-app-compat.js')
importScripts('https://www.gstatic.com/firebasejs/10.14.1/firebase-messaging-compat.js')

firebase.initializeApp({
  apiKey: "AIzaSyDwAeORMe8Cyx9ewIzLF2g3qJSa6qRPaBc",
  authDomain: "blog-but-different.firebaseapp.com",
  projectId: "blog-but-different",
  storageBucket: "blog-but-different.firebasestorage.app",
  messagingSenderId: "572427222580",
  appId: "1:572427222580:web:fddb92bbb43a53e893d461",
})

const messaging = firebase.messaging()

messaging.onBackgroundMessage((payload) => {
  const title = payload.notification?.title || 'New message'
  const body = payload.notification?.body || ''
  self.registration.showNotification(title, {
    body,
    icon: '/favicon.webp',
  })
})
