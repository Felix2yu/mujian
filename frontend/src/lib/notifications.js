export async function requestPermission() {
  if (!('Notification' in window)) return false;
  if (Notification.permission === 'granted') return true;
  if (Notification.permission === 'denied') return false;
  const result = await Notification.requestPermission();
  return result === 'granted';
}

export function showNotification(title, options = {}) {
  if (Notification.permission !== 'granted') return;
  const notification = new Notification(title, {
    icon: '/icons/icon-192.svg',
    badge: '/icons/icon-192.svg',
    vibrate: [200, 100, 200],
    ...options
  });
  notification.onclick = () => {
    window.focus();
    if (options.url) {
      window.location.href = options.url;
    }
    notification.close();
  };
  return notification;
}

export function checkUpcomingShows(shows) {
  const now = new Date();
  const reminders = [];

  shows.forEach(show => {
    if (show.status !== 'normal' && show.status !== 'pending_tickets') return;

    const showDate = new Date(show.date);
    const diffMs = showDate - now;
    const diffHours = diffMs / (1000 * 60 * 60);
    const diffDays = diffMs / (1000 * 60 * 60 * 24);

    let reminderTime = null;
    let message = '';

    if (diffDays > 0 && diffDays <= 1) {
      reminderTime = '明天';
      message = `${show.venue ? show.venue + ' · ' : ''}${formatTime(showDate)}`;
    } else if (diffHours > 0 && diffHours <= 3) {
      reminderTime = '3小时后';
      message = `${show.venue ? show.venue + ' · ' : ''}${formatTime(showDate)}`;
    } else if (diffHours > 0 && diffHours <= 1) {
      reminderTime = '1小时后';
      message = `${show.venue ? show.venue + ' · ' : ''}${formatTime(showDate)}`;
    }

    if (reminderTime) {
      reminders.push({
        show,
        reminderTime,
        message,
        url: `/shows/${show.id}`
      });
    }
  });

  return reminders;
}

function formatTime(date) {
  const h = String(date.getHours()).padStart(2, '0');
  const m = String(date.getMinutes()).padStart(2, '0');
  return `${h}:${m}`;
}

let notifiedIds = new Set();

export function sendReminders(reminders) {
  reminders.forEach(reminder => {
    const key = `${reminder.show.id}-${reminder.reminderTime}`;
    if (notifiedIds.has(key)) return;
    notifiedIds.add(key);

    showNotification(`即将开始: ${reminder.show.name}`, {
      body: reminder.message,
      url: reminder.url,
      tag: key,
      requireInteraction: true
    });
  });
}

export function loadNotifiedIds() {
  try {
    const stored = localStorage.getItem('notified_ids');
    if (stored) {
      notifiedIds = new Set(JSON.parse(stored));
    }
  } catch {}
}

export function saveNotifiedIds() {
  try {
    localStorage.setItem('notified_ids', JSON.stringify([...notifiedIds]));
  } catch {}
}

let checkInterval = null;

export function startReminderCheck(showsGetter) {
  loadNotifiedIds();

  if (checkInterval) clearInterval(checkInterval);

  checkInterval = setInterval(async () => {
    try {
      const shows = await showsGetter();
      const reminders = checkUpcomingShows(shows);
      sendReminders(reminders);
      saveNotifiedIds();
    } catch (e) {
      console.error('Reminder check failed:', e);
    }
  }, 60 * 1000);

  const shows = showsGetter();
  if (shows instanceof Promise) {
    shows.then(s => {
      const reminders = checkUpcomingShows(s);
      sendReminders(reminders);
      saveNotifiedIds();
    });
  }
}

export function stopReminderCheck() {
  if (checkInterval) {
    clearInterval(checkInterval);
    checkInterval = null;
  }
}
